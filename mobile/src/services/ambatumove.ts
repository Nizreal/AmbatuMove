import {
  Constraints,
  Exercise,
  Plan,
  PlanItem,
  labels,
  uid,
} from "../domain/workout";
import { exercisePhoto } from "../data/exercise-media";
import { Platform } from "react-native";

const AMBATUMOVE_PUBLIC_API = "https://nizdri.stei.fun/api/v1";
const AMBATUFIT_PUBLIC_API = "https://ambatufit.stei.fun/api/v1";
const AMBATUMOVE_WEB_PROXY = "/__dev_proxy/ambatumove";
const AMBATUFIT_WEB_PROXY = "/__dev_proxy/ambatufit";
const trimTrailingSlashes = (value: string) => value.replace(/\/+$/, "");
export function resolveApiBase(
  platform: string,
  development: boolean,
  override: string | undefined,
  publicUrl: string,
  webProxyUrl: string,
) {
  if (development && platform === "web") return webProxyUrl;
  return trimTrailingSlashes(override?.trim() || publicUrl);
}
export const AMBATUMOVE_API = resolveApiBase(
  Platform.OS,
  __DEV__,
  process.env.EXPO_PUBLIC_AMBATUMOVE_API_URL,
  AMBATUMOVE_PUBLIC_API,
  AMBATUMOVE_WEB_PROXY,
);
const AMBATUFIT_API = resolveApiBase(
  Platform.OS,
  __DEV__,
  process.env.EXPO_PUBLIC_AMBATUFIT_API_URL,
  AMBATUFIT_PUBLIC_API,
  AMBATUFIT_WEB_PROXY,
);
export const API_CLIENT_BASES = {
  ambatumove: AMBATUMOVE_API,
  ambatufit: AMBATUFIT_API,
};

export type ApiErrorKind =
  | "network"
  | "timeout"
  | "http"
  | "authentication"
  | "missing-data"
  | "invalid-response";

export class ApiError extends Error {
  constructor(
    readonly kind: ApiErrorKind,
    message: string,
    readonly url?: string,
    readonly status?: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function describeApiError(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return error instanceof Error ? error.message : "Terjadi kesalahan API.";
  }
  const endpoint = error.url ? ` (${error.url})` : "";
  switch (error.kind) {
    case "network":
      return `Tidak dapat terhubung ke API${endpoint}. Periksa jaringan atau proxy development.`;
    case "timeout":
      return `API tidak merespons dalam batas waktu${endpoint}.`;
    case "authentication":
      return `Autentikasi atau izin API ditolak${error.status ? ` (HTTP ${error.status})` : ""}${error.code ? ` · ${error.code}` : ""}. ${error.message}`;
    case "missing-data":
      return `Data atau endpoint API tidak ditemukan${error.status ? ` (HTTP ${error.status})` : ""}${error.code ? ` · ${error.code}` : ""}. ${error.message}`;
    case "invalid-response":
      return `Respons API tidak sesuai format${endpoint}. ${error.message}`;
    case "http":
      return `API merespons HTTP ${error.status ?? "?"}${error.code ? ` · ${error.code}` : ""}. ${error.message}`;
  }
}

type Envelope<T> = { data: T; meta?: Record<string, unknown> };
export type EquipmentOption = {
  id: string;
  label: string;
  category?: string;
};
export type MuscleOption = { id: string; label: string };
export type GymOption = {
  id: string;
  name: string;
  address?: { formatted?: string; city?: string };
  verificationStatus?: string;
};

async function request<T>(
  path: string,
  init?: RequestInit,
  baseUrl = AMBATUMOVE_API,
): Promise<T> {
  const url = `${baseUrl}${path}`;
  const method = init?.method ?? "GET";
  const controller = new AbortController();
  let didTimeout = false;
  const timeout = setTimeout(() => {
    didTimeout = true;
    controller.abort();
  }, 15000);
  try {
    const headers = new Headers(init?.headers);
    if (!headers.has("Accept")) headers.set("Accept", "application/json");
    if (init?.body && !headers.has("Content-Type"))
      headers.set("Content-Type", "application/json");
    if (__DEV__) console.info(`[api] ${method} ${url}`);
    const response = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers,
    });
    const bodyText = await response.text();
    let payload: unknown;
    let invalidJson = false;
    if (bodyText) {
      try {
        payload = JSON.parse(bodyText);
      } catch {
        invalidJson = true;
      }
    }
    if (!response.ok) {
      const envelope = payload as
        | { error?: { code?: string; message?: string }; message?: string }
        | undefined;
      const code = envelope?.error?.code;
      const message =
        envelope?.error?.message ??
        envelope?.message ??
        ((invalidJson && bodyText ? bodyText.slice(0, 180) : "") ||
          `Permintaan ${method} gagal.`);
      const kind: ApiErrorKind =
        code === "DEV_PROXY_UPSTREAM_ERROR"
          ? "network"
          : response.status === 401 || response.status === 403
            ? "authentication"
            : response.status === 404
              ? "missing-data"
              : "http";
      throw new ApiError(kind, message, url, response.status, code);
    }
    if (invalidJson) {
      throw new ApiError(
        "invalid-response",
        "API mengirim isi yang bukan JSON.",
        url,
        response.status,
      );
    }
    if (payload === undefined)
      throw new ApiError(
        "missing-data",
        "API mengirim respons kosong.",
        url,
        response.status,
      );
    if (__DEV__)
      console.info(`[api] ${method} ${url} -> HTTP ${response.status}`);
    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (didTimeout)
      throw new ApiError("timeout", "Permintaan melewati batas waktu.", url);
    throw new ApiError(
      "network",
      error instanceof Error ? error.message : "Koneksi jaringan gagal.",
      url,
    );
  } finally {
    clearTimeout(timeout);
  }
}

function requireData<T>(payload: Envelope<T> | undefined, url: string): T {
  if (!payload || !Object.prototype.hasOwnProperty.call(payload, "data"))
    throw new ApiError(
      "missing-data",
      "Respons sukses tidak memiliki field data.",
      url,
    );
  return payload.data;
}

const muscleAlias: Record<string, string> = {
  pectorals: "chest",
  pectoral: "chest",
  latissimus_dorsi: "back",
  lats: "back",
  biceps_brachii: "biceps",
  triceps_brachii: "triceps",
  deltoids: "shoulders",
  abdominals: "core",
};
const prettify = (id: string) =>
  id
    .split(/[_\s]+/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");

export function mapExercise(raw: Record<string, any>): Exercise {
  const muscle = raw.primary_muscles?.[0] ?? raw.category ?? "general";
  const muscleId = muscleAlias[muscle] ?? muscle;
  if (!labels[muscleId]) labels[muscleId] = prettify(muscleId);
  const equipment = Array.isArray(raw.required_equipment)
    ? raw.required_equipment.filter((id: unknown) => typeof id === "string")
    : [];
  const minimumSpace = raw.minimum_space;
  return {
    id: String(raw.id),
    name: String(raw.name),
    muscle: muscleAlias[muscle] ?? muscle,
    equipment,
    space:
      minimumSpace === "small"
        ? "small"
        : minimumSpace === "large"
          ? "large"
          : "medium",
    environments: Array.isArray(raw.environments)
      ? raw.environments.filter((value: string) =>
          ["home", "gym", "indoor", "outdoor"].includes(value),
        )
      : [],
    level: ["beginner", "intermediate", "advanced"].includes(raw.difficulty)
      ? raw.difficulty
      : "beginner",
    timed: raw.measurement_type === "duration",
    instructions: Array.isArray(raw.instructions)
      ? raw.instructions.filter((line: unknown) => typeof line === "string")
      : [],
    reference: `${AMBATUMOVE_API}/exercises/${encodeURIComponent(String(raw.id))}`,
    description:
      typeof raw.description === "string" ? raw.description : undefined,
    cautions: Array.isArray(raw.cautions) ? raw.cautions : [],
    category: raw.category,
    measurement: raw.measurement_type,
    movementPattern: raw.movement_pattern,
    image: exercisePhoto(String(raw.id)),
  };
}

export async function getExercises(): Promise<Exercise[]> {
  const rawItems: Record<string, any>[] = [];
  let offset = 0;
  let hasMore = true;
  while (hasMore) {
    const url = `${AMBATUMOVE_API}/exercises?limit=100&offset=${offset}&sort=name`;
    const response = await request<Envelope<Record<string, any>[]>>(
      `/exercises?limit=100&offset=${offset}&sort=name`,
    );
    const page = requireData(response, url);
    if (!Array.isArray(page))
      throw new ApiError(
        "invalid-response",
        "Format katalog tidak valid.",
        url,
      );
    rawItems.push(...page);
    const pagination = response.meta?.pagination as
      { has_more?: boolean; limit?: number } | undefined;
    hasMore = pagination?.has_more ?? page.length === 100;
    if ((!page.length && hasMore) || rawItems.length > 5000)
      throw new ApiError(
        "invalid-response",
        "Paginasi katalog tidak valid.",
        url,
      );
    if (!page.length) break;
    offset += pagination?.limit ?? 100;
  }
  if (!rawItems.length)
    throw new ApiError(
      "missing-data",
      "Katalog gerakan AmbatuMove belum berisi data.",
      `${AMBATUMOVE_API}/exercises`,
    );
  const exercises = rawItems.map(mapExercise);
  if (
    new Set(exercises.map((exercise) => exercise.id)).size !== exercises.length
  )
    throw new ApiError(
      "invalid-response",
      "API mengirim ID gerakan yang duplikat.",
      `${AMBATUMOVE_API}/exercises`,
    );
  return exercises;
}

export async function getExerciseDetails(id: string): Promise<Exercise> {
  const result = await request<Envelope<Record<string, any>>>(
    `/exercises/${encodeURIComponent(id)}`,
  );
  const url = `${AMBATUMOVE_API}/exercises/${encodeURIComponent(id)}`;
  const data = requireData(result, url);
  if (!data || typeof data !== "object")
    throw new ApiError("missing-data", "Detail gerakan tidak tersedia.", url);
  return mapExercise(data);
}

export async function getEquipment(): Promise<EquipmentOption[]> {
  const url = `${AMBATUMOVE_API}/equipment`;
  const result = await request<Envelope<Record<string, any>[]>>("/equipment");
  const data = requireData(result, url);
  if (!Array.isArray(data))
    throw new ApiError(
      "invalid-response",
      "Format daftar alat tidak valid.",
      url,
    );
  const equipment = data
    .filter((item) => item.active !== false)
    .map((item) => ({
      id: String(item.id),
      label: String(item.label),
      category: item.category,
    }));
  for (const item of equipment) labels[item.id] = item.label;
  return equipment;
}

export async function getMuscleGroups(): Promise<MuscleOption[]> {
  const url = `${AMBATUMOVE_API}/muscle-groups`;
  const result =
    await request<Envelope<Record<string, any>[]>>("/muscle-groups");
  const data = requireData(result, url);
  if (!Array.isArray(data))
    throw new ApiError(
      "invalid-response",
      "Format kelompok otot tidak valid.",
      url,
    );
  return data.map((item) => ({
    id: String(item.id),
    label: String(item.label ?? prettify(String(item.id))),
  }));
}

export async function getGyms(): Promise<GymOption[]> {
  const url = `${AMBATUMOVE_API}/gyms?limit=100&offset=0`;
  const result = await request<Envelope<Record<string, any>[]>>(
    "/gyms?limit=100&offset=0",
  );
  const data = requireData(result, url);
  if (!Array.isArray(data))
    throw new ApiError(
      "invalid-response",
      "Format daftar gym tidak valid.",
      url,
    );
  return data.map((item) => ({
    id: String(item.id),
    name: String(item.name),
    address: item.address,
    verificationStatus: item.verification_status,
  }));
}

export async function getGymEquipment(
  gymId: string,
): Promise<EquipmentOption[]> {
  const url = `${AMBATUMOVE_API}/gyms/${encodeURIComponent(gymId)}/equipment`;
  const result = await request<Envelope<Record<string, any>[]>>(
    `/gyms/${encodeURIComponent(gymId)}/equipment`,
  );
  const data = requireData(result, url);
  if (!Array.isArray(data))
    throw new ApiError(
      "invalid-response",
      "Format inventaris gym tidak valid.",
      url,
    );
  return data
    .filter(
      (item) => item.status !== "unavailable" && item.status !== "inactive",
    )
    .map((item) => {
      const id = String(item.equipment_id ?? item.id ?? "");
      const label = String(item.label ?? labels[id] ?? prettify(id));
      labels[id] = label;
      return { id, label };
    })
    .filter((item) => item.id.length > 0);
}

type WorkoutRequest = {
  goal: "general_fitness" | "strength" | "endurance";
  workout_type: "strength" | "cardio" | "mixed";
  duration_minutes: number;
  experience_level: Constraints["experience"];
  environment: Constraints["environment"];
  space: Constraints["space"];
  available_equipment: string[];
  equipment_source: "explicit" | "gym_inventory";
  gym_id?: string;
  focus_muscles?: string[];
  intensity: Constraints["intensity"];
  save: false;
  seed: number;
};
type RawPrescription = {
  sets?: number;
  reps?: number;
  duration_seconds?: number;
  rest_seconds?: number;
};
type RawWorkoutItem = {
  item_id: string;
  exercise_id: string;
  prescription: RawPrescription;
};

function workoutRequest(c: Constraints): WorkoutRequest {
  return {
    goal:
      c.goal === "strength"
        ? "strength"
        : c.goal === "cardio"
          ? "endurance"
          : "general_fitness",
    workout_type:
      c.goal === "strength"
        ? "strength"
        : c.goal === "cardio"
          ? "cardio"
          : "mixed",
    duration_minutes: c.durationMinutes,
    experience_level: c.experience,
    environment: c.environment,
    space: c.space,
    available_equipment:
      c.environment === "gym" && c.equipmentSource === "gym_inventory"
        ? []
        : c.equipment,
    equipment_source:
      c.environment === "gym" && c.equipmentSource === "gym_inventory"
        ? "gym_inventory"
        : "explicit",
    ...(c.environment === "gym" && c.gymId ? { gym_id: c.gymId } : {}),
    ...(c.focus !== "full" ? { focus_muscles: focusToGroups(c.focus) } : {}),
    intensity: c.intensity,
    save: false,
    seed: Math.floor(Math.random() * 2_000_000_000),
  };
}

function focusToGroups(focus: string): string[] {
  if (focus === "upper")
    return ["chest", "back", "shoulders", "biceps", "triceps"];
  if (focus === "lower")
    return ["quadriceps", "hamstrings", "glutes", "calves"];
  if (focus === "core") return ["core"];
  return [focus];
}

export async function generateWorkout(
  constraints: Constraints,
  catalog: Exercise[],
): Promise<Plan> {
  const url = `${AMBATUMOVE_API}/workouts/generate`;
  const result = await request<Envelope<Record<string, any>>>(
    "/workouts/generate",
    {
      method: "POST",
      body: JSON.stringify(workoutRequest(constraints)),
    },
  );
  const raw = requireData(result, url);
  if (!raw || !Array.isArray(raw.sections))
    throw new ApiError("invalid-response", "Format workout tidak valid.", url);
  const indexed = new Map(catalog.map((exercise) => [exercise.id, exercise]));
  const rawItems = raw.sections.flatMap((section: any) =>
    (section.items ?? []).map((item: RawWorkoutItem) => ({
      ...item,
      section: section.type,
    })),
  );
  const missing = [
    ...new Set(rawItems.map((item: any) => item.exercise_id)),
  ].filter((id) => !indexed.has(id));
  await Promise.all(
    missing.map(async (id) => {
      try {
        indexed.set(id, await getExerciseDetails(id));
      } catch {
        /* keep the API workout response intact below */
      }
    }),
  );
  const items: PlanItem[] = rawItems.map((item: any) => {
    const exercise = indexed.get(item.exercise_id) ?? {
      id: item.exercise_id,
      name: prettify(item.exercise_id.replace(/^(ex|act)_/, "")),
      muscle: "general",
      equipment: [],
      space: "small" as const,
      environments: [constraints.environment],
      level: "beginner" as const,
      timed: typeof item.prescription.duration_seconds === "number",
      instructions: [],
      reference: `${AMBATUMOVE_API}/exercises/${encodeURIComponent(item.exercise_id)}`,
    };
    const isTimed = typeof item.prescription.duration_seconds === "number";
    return {
      exercise: { ...exercise, timed: isTimed },
      sets: isTimed ? 1 : Math.max(1, item.prescription.sets ?? 1),
      target: isTimed
        ? item.prescription.duration_seconds!
        : Math.max(1, item.prescription.reps ?? 1),
      rest: Math.max(0, item.prescription.rest_seconds ?? 0),
    };
  });
  if (!items.length)
    throw new ApiError(
      "missing-data",
      "Belum ada rencana yang cocok dengan kondisi tersebut.",
      url,
    );
  const goalLabel =
    raw.goal === "strength"
      ? "Kekuatan"
      : raw.goal === "endurance"
        ? "Daya tahan"
        : "Kebugaran";
  return {
    id: raw.id ?? uid(),
    backendWorkoutId: raw.id,
    backendVersion: raw.version,
    backendEstimatedDurationSeconds: raw.estimated_duration_seconds,
    title: `${goalLabel} · ${raw.constraints?.environment ?? constraints.environment}`,
    source: "recommendation",
    constraints: { ...constraints },
    items,
    createdAt: new Date().toISOString(),
    warnings: Array.isArray(raw.warnings) ? raw.warnings : [],
  };
}

export type Professional = {
  id: string;
  displayName: string;
  specialty: string;
  description?: string;
};
export async function getProfessionals(): Promise<Professional[]> {
  const url = `${AMBATUFIT_API}/professionals`;
  const payload = await request<{
    items?: Record<string, unknown>[];
  }>("/professionals", undefined, AMBATUFIT_API);
  if (!payload || typeof payload !== "object" || !Array.isArray(payload.items))
    throw new ApiError(
      "missing-data",
      "Direktori tidak memiliki field items.",
      url,
    );
  return payload.items
    .filter((item) => item.is_active === true)
    .map((item) => ({
      id: String(item.id),
      displayName: String(item.display_name),
      specialty: String(item.specialty),
      description:
        typeof item.description === "string" ? item.description : undefined,
    }));
}

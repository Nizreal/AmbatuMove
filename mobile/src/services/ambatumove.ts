import {
  Constraints,
  Exercise,
  Plan,
  PlanItem,
  Session,
  labels,
  uid,
} from "../domain/workout";
import { exercisePhoto } from "../data/exercise-media";
import { Platform } from "react-native";

const AMBATUMOVE_PUBLIC_API = "https://nizdri.stei.fun/api/v1";
const AMBATUMOVE_WEB_PROXY = "/__dev_proxy/ambatumove";
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
export const API_CLIENT_BASES = {
  ambatumove: AMBATUMOVE_API,
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
export type ApiUser = {
  id: string;
  email: string;
  role: string;
  created_at?: string;
};
export type TokenPair = {
  user: ApiUser;
  access_token: string;
  refresh_token: string;
  token_type: string;
  access_token_expires_at: string;
  refresh_token_expires_at: string;
};
export type WorkoutLogRecord = {
  id: string;
  session_id: string | null;
  status: "completed" | "partial" | "skipped" | string;
  started_at: string;
  finished_at: string;
  notes?: string;
  items: string[];
  created_at?: string;
};
export type CoachResponse = {
  answer: string;
  safety_classification: "general" | "caution" | "urgent";
  suggested_actions: string[];
  request_id: string;
  limitations: string;
  provider?: string;
  model?: string;
};

async function request<T>(
  path: string,
  init?: RequestInit,
  baseUrl = AMBATUMOVE_API,
  accessToken?: string,
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
    if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
    if (__DEV__) console.info(`[api] ${method} ${url}`);
    const response = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers,
    });
    const bodyText = await response.text();
    if (response.status === 204 && !bodyText) return undefined as T;
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

function requireEnvelopeData<T>(payload: unknown, url: string): T {
  return requireData(payload as Envelope<T> | undefined, url);
}

export async function loginApi(email: string, password: string) {
  const url = `${AMBATUMOVE_API}/auth/login`;
  const payload = await request<Envelope<TokenPair>>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email: email.trim().toLowerCase(), password }),
  });
  const pair = requireData(payload, url);
  if (!pair?.access_token || !pair.refresh_token || !pair.user?.email)
    throw new ApiError("invalid-response", "Data sesi akun tidak lengkap.", url);
  return pair;
}

export async function registerApi(email: string, password: string) {
  const url = `${AMBATUMOVE_API}/auth/register`;
  const payload = await request<Envelope<TokenPair>>("/auth/register", {
    method: "POST",
    body: JSON.stringify({ email: email.trim().toLowerCase(), password }),
  });
  const pair = requireData(payload, url);
  if (!pair?.access_token || !pair.refresh_token || !pair.user?.email)
    throw new ApiError("invalid-response", "Data sesi akun tidak lengkap.", url);
  return pair;
}

export async function refreshApiTokens(refreshToken: string) {
  const url = `${AMBATUMOVE_API}/auth/refresh`;
  const payload = await request<Envelope<TokenPair>>("/auth/refresh", {
    method: "POST",
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  const pair = requireData(payload, url);
  if (!pair?.access_token || !pair.refresh_token || !pair.user?.email)
    throw new ApiError("invalid-response", "Token baru tidak lengkap.", url);
  return pair;
}

export async function getCurrentUser(accessToken: string) {
  const url = `${AMBATUMOVE_API}/me`;
  const payload = await request<Envelope<ApiUser>>("/me", undefined, AMBATUMOVE_API, accessToken);
  const user = requireData(payload, url);
  if (!user?.id || !user.email)
    throw new ApiError("invalid-response", "Profil akun tidak lengkap.", url);
  return user;
}

type FitnessProfile = {
  goal: string;
  experience_level: string;
  preferred_activity_ids: string[];
  equipment: string[];
  environment: string;
  space: string;
  weight_kg?: number | null;
  excluded_ids: string[];
  updated_at?: string;
};

export async function getFitnessProfile(accessToken: string) {
  const path = "/me/fitness-profile";
  const payload = await request<Envelope<FitnessProfile>>(path, undefined, AMBATUMOVE_API, accessToken);
  return requireEnvelopeData<FitnessProfile>(payload, `${AMBATUMOVE_API}${path}`);
}

export async function saveFitnessPreferences(accessToken: string, constraints: Constraints) {
  let previous: FitnessProfile | undefined;
  try {
    previous = await getFitnessProfile(accessToken);
  } catch (error) {
    if (!(error instanceof ApiError) || error.kind !== "missing-data" || error.status !== 404) throw error;
  }
  const path = "/me/fitness-profile";
  const body: FitnessProfile = {
    goal: constraints.goal === "strength" ? "strength" : constraints.goal === "cardio" ? "endurance" : "general_fitness",
    experience_level: constraints.experience,
    preferred_activity_ids: previous?.preferred_activity_ids ?? [],
    equipment: constraints.environment === "gym" && constraints.equipmentSource === "gym_inventory" ? previous?.equipment ?? [] : constraints.equipment,
    environment: constraints.environment,
    space: constraints.space,
    ...(previous?.weight_kg !== undefined ? { weight_kg: previous.weight_kg } : {}),
    excluded_ids: previous?.excluded_ids ?? [],
  };
  await request<Envelope<FitnessProfile>>(path, { method: "PUT", body: JSON.stringify(body) }, AMBATUMOVE_API, accessToken);
  if (!(constraints.environment === "gym" && constraints.equipmentSource === "gym_inventory")) {
    const equipmentPath = "/me/equipment";
    await request<Envelope<{ configured: boolean; equipment: string[] }>>(equipmentPath, {
      method: "PUT",
      body: JSON.stringify({ equipment: constraints.equipment }),
    }, AMBATUMOVE_API, accessToken);
  }
}

export async function logoutApi(accessToken: string) {
  await request<void>("/auth/logout", { method: "POST" }, AMBATUMOVE_API, accessToken);
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
  const rows: Record<string, any>[] = [];
  let offset = 0;
  for (let pageNumber = 0; pageNumber < 100; pageNumber += 1) {
    const path = `/gyms?limit=100&offset=${offset}`;
    const url = `${AMBATUMOVE_API}${path}`;
    const result = await request<Envelope<Record<string, any>[]>>(path);
    const data = requireData(result, url);
    if (!Array.isArray(data))
      throw new ApiError("invalid-response", "Format daftar gym tidak valid.", url);
    rows.push(...data);
    const pagination = result.meta?.pagination as { has_more?: boolean; limit?: number } | undefined;
    const hasMore = pagination?.has_more ?? data.length === 100;
    if (!hasMore) break;
    if (!data.length) throw new ApiError("invalid-response", "Paginasi direktori gym tidak valid.", url);
    offset += pagination?.limit ?? 100;
    if (pageNumber === 99) throw new ApiError("invalid-response", "Direktori gym melebihi batas paginasi.", url);
  }
  return rows.map((item) => ({
    id: String(item.id),
    name: String(item.name),
    address: item.address,
    verificationStatus: item.verification_status,
  }));
}

const gymEquipmentCache = new Map<
  string,
  { expiresAt: number; items: EquipmentOption[] }
>();
const gymEquipmentRequests = new Map<string, Promise<EquipmentOption[]>>();
const GYM_EQUIPMENT_CACHE_MS = 60_000;

export function getGymEquipment(
  gymId: string,
): Promise<EquipmentOption[]> {
  const cached = gymEquipmentCache.get(gymId);
  if (cached && cached.expiresAt > Date.now())
    return Promise.resolve(cached.items);
  const existing = gymEquipmentRequests.get(gymId);
  if (existing) return existing;

  const pending = loadGymEquipment(gymId).then((items) => {
    gymEquipmentCache.set(gymId, {
      expiresAt: Date.now() + GYM_EQUIPMENT_CACHE_MS,
      items,
    });
    return items;
  }).finally(() => {
    gymEquipmentRequests.delete(gymId);
  });
  gymEquipmentRequests.set(gymId, pending);
  return pending;
}

async function loadGymEquipment(gymId: string): Promise<EquipmentOption[]> {
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
  save: boolean;
  seed?: number;
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

function workoutRequest(
  c: Constraints,
  options: { save: boolean; seed?: number },
): WorkoutRequest {
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
    save: options.save,
    ...(options.seed === undefined ? {} : { seed: options.seed }),
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
  accessToken: string,
  options: { save?: boolean; seed?: number } = {},
): Promise<Plan> {
  const url = `${AMBATUMOVE_API}/workouts/generate`;
  const payload = workoutRequest(constraints, {
    save: options.save ?? true,
    ...(options.seed === undefined ? {} : { seed: options.seed }),
  });
  let result: Envelope<Record<string, any>>;
  try {
    result = await request<Envelope<Record<string, any>>>(
      "/workouts/generate",
      { method: "POST", body: JSON.stringify(payload) },
      AMBATUMOVE_API,
      accessToken,
    );
  } catch (error) {
    if (__DEV__) {
      const apiError = error instanceof ApiError ? error : undefined;
      console.warn("[workout] generation rejected", {
        url,
        request: payload,
        response: apiError
          ? { status: apiError.status, code: apiError.code, message: apiError.message }
          : { message: error instanceof Error ? error.message : "Unknown error" },
      });
    }
    throw error;
  }
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
    missing.map(async (id) => indexed.set(id, await getExerciseDetails(id))),
  );
  const items: PlanItem[] = rawItems.map((item: any) => {
    const exercise = indexed.get(item.exercise_id);
    if (!exercise)
      throw new ApiError(
        "missing-data",
        `Gerakan ${item.exercise_id} tidak ditemukan di katalog aktif.`,
        url,
      );
    const isTimed = typeof item.prescription.duration_seconds === "number";
    return {
      exercise: { ...exercise, timed: isTimed },
      sets: isTimed ? 1 : Math.max(1, item.prescription.sets ?? 1),
      target: isTimed
        ? item.prescription.duration_seconds!
        : Math.max(1, item.prescription.reps ?? 1),
      rest: Math.max(0, item.prescription.rest_seconds ?? 0),
      backendItemId: item.item_id,
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
    backendWorkoutId: raw.saved === false ? undefined : raw.id,
    backendVersion: raw.version,
    backendEstimatedDurationSeconds: raw.estimated_duration_seconds,
    backendSeed: raw.generation?.seed,
    title: `${goalLabel} · ${raw.constraints?.environment ?? constraints.environment}`,
    source: "recommendation",
    constraints: { ...constraints },
    items,
    createdAt: new Date().toISOString(),
    warnings: Array.isArray(raw.warnings) ? raw.warnings : [],
  };
}

export async function getGymDetails(gymId: string): Promise<Record<string, unknown>> {
  const url = `${AMBATUMOVE_API}/gyms/${encodeURIComponent(gymId)}`;
  const result = await request<Envelope<Record<string, unknown>>>(
    `/gyms/${encodeURIComponent(gymId)}`,
  );
  return requireData(result, url);
}

export async function getExerciseAlternatives(
  original: Exercise,
  constraints: Constraints,
): Promise<string[]> {
  const path = `/exercises/${encodeURIComponent(original.id)}/alternatives`;
  const payload = await request<Envelope<{ alternatives?: { exercise_id: string }[] }>>(path, {
    method: "POST",
    body: JSON.stringify({
      available_equipment: constraints.equipment,
      experience_level: constraints.experience,
      environment: constraints.environment,
      space: constraints.space,
      goal: constraints.goal === "strength" ? "strength" : constraints.goal === "cardio" ? "endurance" : "general_fitness",
      max_results: 10,
      excluded_exercise_ids: [],
    }),
  });
  const data = requireEnvelopeData<{ alternatives?: { exercise_id: string }[] }>(payload, `${AMBATUMOVE_API}${path}`);
  if (!Array.isArray(data.alternatives))
    throw new ApiError("invalid-response", "Daftar alternatif tidak valid.", `${AMBATUMOVE_API}${path}`);
  return data.alternatives.map((item) => String(item.exercise_id));
}

export async function getWorkoutLogs(accessToken: string): Promise<WorkoutLogRecord[]> {
  const all: WorkoutLogRecord[] = [];
  let offset = 0;
  for (let pageNumber = 0; pageNumber < 100; pageNumber += 1) {
    const path = `/workout-logs?limit=100&offset=${offset}`;
    const url = `${AMBATUMOVE_API}${path}`;
    const payload = await request<Envelope<Record<string, unknown>[]>>(path, undefined, AMBATUMOVE_API, accessToken);
    const page = requireData(payload, url);
    if (!Array.isArray(page))
      throw new ApiError("invalid-response", "Format riwayat latihan tidak valid.", url);
    all.push(...page.map((raw) => ({
      id: String(raw.id ?? ""),
      session_id: typeof raw.session_id === "string" ? raw.session_id : null,
      status: String(raw.status ?? "unknown"),
      started_at: String(raw.started_at ?? ""),
      finished_at: String(raw.finished_at ?? ""),
      notes: typeof raw.notes === "string" ? raw.notes : undefined,
      items: Array.isArray(raw.items) ? raw.items.map(String) : [],
      created_at: typeof raw.created_at === "string" ? raw.created_at : undefined,
    })));
    const pagination = payload.meta?.pagination as { has_more?: boolean; limit?: number } | undefined;
    if (!(pagination?.has_more ?? page.length === 100)) return all;
    if (page.length === 0)
      throw new ApiError("invalid-response", "Paginasi riwayat tidak valid.", url);
    offset += pagination?.limit ?? 100;
  }
  throw new ApiError("invalid-response", "Riwayat melebihi batas paginasi.", `${AMBATUMOVE_API}/workout-logs`);
}

export async function getWorkoutLog(accessToken: string, logId: string) {
  const path = `/workout-logs/${encodeURIComponent(logId)}`;
  const payload = await request<Envelope<WorkoutLogRecord>>(path, undefined, AMBATUMOVE_API, accessToken);
  return requireEnvelopeData<WorkoutLogRecord>(payload, `${AMBATUMOVE_API}${path}`);
}

function sessionLogItems(session: Session) {
  return session.plan.items.flatMap((item, index) => {
    const completedSets = session.actual[index].filter((status) => status === "completed").length;
    if (completedSets === 0) return [];
    return [{
      exercise_id: item.exercise.id,
      ...(!item.exercise.timed ? { performed_sets: Array.from({ length: completedSets }, () => ({ reps: item.target })) } : {}),
      ...(item.exercise.timed ? { actual_duration_seconds: completedSets * item.target } : {}),
    }];
  });
}

export async function saveCompletedSession(accessToken: string, session: Session) {
  const completedSets = session.actual.flat().filter((status) => status === "completed").length;
  const totalSets = session.actual.flat().length;
  const status = completedSets === 0 ? "skipped" : completedSets === totalSets ? "completed" : "partial";
  const startedAt = session.startedAt;
  const finishedAt = session.finishedAt ?? new Date().toISOString();
  const idempotencyKey = `ambatumove-${session.id}`;
  if (status !== "skipped" && session.plan.backendWorkoutId && session.plan.items.every((item) => item.backendItemId)) {
    const path = `/workouts/${encodeURIComponent(session.plan.backendWorkoutId)}/complete`;
    const payload = await request<Envelope<WorkoutLogRecord>>(path, {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({
        status,
        started_at: startedAt,
        finished_at: finishedAt,
        items: session.plan.items.flatMap((item, index) => {
          const completedSets = session.actual[index].filter((setStatus) => setStatus === "completed").length;
          if (completedSets === 0) return [];
          return [{
            item_id: item.backendItemId,
            exercise_id: item.exercise.id,
            ...(!item.exercise.timed ? { performed_sets: Array.from({ length: completedSets }, () => ({ reps: item.target })) } : {}),
            ...(item.exercise.timed ? { actual_duration_seconds: completedSets * item.target } : {}),
          }];
        }),
      }),
    }, AMBATUMOVE_API, accessToken);
    return requireEnvelopeData<WorkoutLogRecord>(payload, `${AMBATUMOVE_API}${path}`);
  }
  const path = "/workout-logs/manual";
  const payload = await request<Envelope<WorkoutLogRecord>>(path, {
    method: "POST",
    headers: { "Idempotency-Key": idempotencyKey },
    body: JSON.stringify({
      status,
      started_at: startedAt,
      finished_at: finishedAt,
      notes: session.plan.title.slice(0, 2000),
      items: sessionLogItems(session),
    }),
  }, AMBATUMOVE_API, accessToken);
  return requireEnvelopeData<WorkoutLogRecord>(payload, `${AMBATUMOVE_API}${path}`);
}

const progressionRequests = new Map<
  string,
  Promise<{ lookback_days?: number; items?: Record<string, unknown>[] }>
>();

export function getProgression(accessToken: string, timezone: string) {
  const path = `/me/progression?days=90&limit=20&timezone=${encodeURIComponent(timezone)}`;
  const key = `${accessToken}\n${timezone}`;
  const existing = progressionRequests.get(key);
  if (existing) return existing;
  const pending = request<Envelope<{ lookback_days?: number; items?: Record<string, unknown>[] }>>(
    path,
    undefined,
    AMBATUMOVE_API,
    accessToken,
  ).then((payload) =>
    requireEnvelopeData<{ lookback_days?: number; items?: Record<string, unknown>[] }>(payload, `${AMBATUMOVE_API}${path}`),
  ).finally(() => {
    progressionRequests.delete(key);
  });
  progressionRequests.set(key, pending);
  return pending;
}

export async function chatWithCoach(
  accessToken: string,
  message: string,
  consentExternalProcessing: boolean,
  previousTurns: { role: "user" | "assistant"; content: string }[] = [],
) {
  const path = "/coach/chat";
  const payload = await request<Envelope<CoachResponse>>(path, {
    method: "POST",
    body: JSON.stringify({
      message: message.trim(),
      consent_external_processing: consentExternalProcessing,
      previous_turns: previousTurns.slice(-6).map((turn) => ({
        role: turn.role,
        content: turn.content.slice(0, 1000),
      })),
      include_profile_context: false,
      include_training_history: false,
    }),
  }, AMBATUMOVE_API, accessToken);
  const data = requireEnvelopeData<CoachResponse>(payload, `${AMBATUMOVE_API}${path}`);
  if (!data.answer || !["general", "caution", "urgent"].includes(data.safety_classification))
    throw new ApiError("invalid-response", "Format jawaban coach tidak valid.", `${AMBATUMOVE_API}${path}`);
  return data;
}

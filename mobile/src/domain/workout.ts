export type Level = "beginner" | "intermediate" | "advanced";
export type Space = "small" | "medium" | "large";
export type Muscle = string;
export type Exercise = {
  id: string;
  name: string;
  muscle: Muscle;
  equipment: string[];
  space: Space;
  environments: ("home" | "gym" | "indoor" | "outdoor")[];
  level: Level;
  timed: boolean;
  instructions: string[];
  image?: number;
  reference?: string;
  description?: string;
  cautions?: string[];
  category?: string;
  measurement?: string;
  movementPattern?: string;
};
export type Constraints = {
  goal: "strength" | "cardio" | "general";
  focus: string;
  experience: Level;
  durationMinutes: number;
  environment: "home" | "gym" | "indoor" | "outdoor";
  space: Space | "unrestricted";
  equipment: string[];
  gymId?: string;
  equipmentSource?: "gym_inventory" | "explicit";
  intensity: "low" | "moderate" | "vigorous";
};
export type PlanItem = {
  exercise: Exercise;
  sets: number;
  target: number;
  rest: number;
};
export type Plan = {
  id: string;
  title: string;
  source: "recommendation" | "manual";
  constraints?: Constraints;
  items: PlanItem[];
  createdAt: string;
  backendWorkoutId?: string | null;
  backendVersion?: number;
  backendEstimatedDurationSeconds?: number;
  warnings?: string[];
};
export type SetStatus = "pending" | "completed" | "skipped";
export type Session = {
  id: string;
  plan: Plan;
  actual: SetStatus[][];
  startedAt: string;
  finishedAt?: string;
  outcome?: "completed" | "partial";
  savedAt?: string;
};
export const CALORIE_REFERENCE_WEIGHT_KG = 70;
const muscleMet: Record<Muscle, number> = {
  chest: 3.5,
  back: 3.5,
  biceps: 3.5,
  triceps: 3.5,
  shoulders: 3.5,
  legs: 5,
  core: 3.8,
  cardio: 7,
};
export const defaults: Constraints = {
  goal: "general",
  focus: "full",
  experience: "beginner",
  durationMinutes: 20,
  environment: "home",
  space: "small",
  equipment: ["bodyweight"],
  intensity: "moderate",
};
export const labels: Record<string, string> = {
  strength: "Kekuatan",
  cardio: "Kardio",
  general: "Kebugaran",
  full: "Seluruh tubuh",
  upper: "Tubuh atas",
  lower: "Tubuh bawah",
  core: "Perut & inti",
  chest: "Dada",
  back: "Punggung",
  biceps: "Biseps",
  triceps: "Triseps",
  shoulders: "Bahu",
  legs: "Kaki",
  beginner: "Pemula",
  intermediate: "Menengah",
  advanced: "Mahir",
  indoor: "Dalam ruangan",
  outdoor: "Luar ruangan",
  small: "Kecil",
  medium: "Sedang",
  large: "Luas",
  mat: "Matras",
  bodyweight: "Berat tubuh",
  quadriceps: "Paha depan",
  hamstrings: "Paha belakang",
  glutes: "Gluteus",
  calves: "Betis",
  forearms: "Lengan bawah",
  dumbbell: "Dumbbell",
  band: "Resistance band",
  chair: "Kursi",
  none: "Tanpa alat",
};
export const uid = () =>
  `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
const levels: Level[] = ["beginner", "intermediate", "advanced"];
const spaces: Space[] = ["small", "medium", "large"];
export function compatible(e: Exercise, c: Constraints) {
  return (
    e.equipment.every((x) => c.equipment.includes(x)) &&
    (c.space === "unrestricted" ||
      spaces.indexOf(e.space) <= spaces.indexOf(c.space as Space)) &&
    levels.indexOf(e.level) <= levels.indexOf(c.experience) &&
    e.environments.includes(c.environment)
  );
}
export function validateConstraints(c: Constraints) {
  return !Number.isInteger(c.durationMinutes) ||
    c.durationMinutes < 5 ||
    c.durationMinutes > 90
    ? "Pilih durasi antara 5–90 menit."
    : c.environment === "gym" && !c.gymId
      ? "Pilih gym untuk menggunakan lingkungan gym."
      : "";
}
export function validateItem(p: PlanItem) {
  return (
    Number.isInteger(p.sets) &&
    p.sets >= 1 &&
    p.sets <= 10 &&
    Number.isInteger(p.target) &&
    p.target >= 1 &&
    p.target <= (p.exercise.timed ? 600 : 100) &&
    p.rest >= 0 &&
    p.rest <= 180
  );
}
export const estimate = (items: PlanItem[]) =>
  Math.ceil(
    items.reduce(
      (n, p) =>
        n + p.sets * ((p.exercise.timed ? p.target : p.target * 3) + p.rest),
      0,
    ) / 60,
  );
export function itemFor(exercise: Exercise, c?: Constraints): PlanItem {
  return {
    exercise,
    sets:
      c?.experience === "advanced"
        ? 4
        : c?.experience === "intermediate"
          ? 3
          : 2,
    target: exercise.timed
      ? c?.goal === "cardio"
        ? 40
        : 30
      : c?.goal === "strength"
        ? 8
        : 12,
    rest: c?.goal === "strength" ? 60 : 30,
  };
}
export const alternatives = (catalog: Exercise[], plan: Plan, index: number) =>
  catalog.filter(
    (e) =>
      e.muscle === plan.items[index].exercise.muscle &&
      !plan.items.some((p) => p.exercise.id === e.id) &&
      (!plan.constraints || compatible(e, plan.constraints)),
  );
export function begin(plan: Plan): Session {
  return {
    id: uid(),
    plan: JSON.parse(JSON.stringify(plan)),
    actual: plan.items.map((p) => Array<SetStatus>(p.sets).fill("pending")),
    startedAt: new Date().toISOString(),
  };
}
export function mark(
  s: Session,
  exercise: number,
  set: number,
  status: SetStatus,
): Session {
  if (s.finishedAt) return s;
  return {
    ...s,
    actual: s.actual.map((row, i) =>
      i === exercise ? row.map((v, j) => (j === set ? status : v)) : row,
    ),
  };
}
export function finish(s: Session): Session {
  return s.finishedAt
    ? s
    : {
        ...s,
        finishedAt: new Date().toISOString(),
        outcome: s.actual.every((row) => row.every((x) => x === "completed"))
          ? "completed"
          : "partial",
      };
}
export function counts(s: Session) {
  const all = s.actual.flat();
  return {
    completed: all.filter((x) => x === "completed").length,
    skipped: all.filter((x) => x === "skipped").length,
    pending: all.filter((x) => x === "pending").length,
    total: all.length,
  };
}
export function estimatedCalories(
  item: PlanItem,
  completedSets: number,
  weightKg = CALORIE_REFERENCE_WEIGHT_KG,
) {
  const activeSeconds =
    Math.max(0, completedSets) * item.target * (item.exercise.timed ? 1 : 3);
  const kcal =
    ((muscleMet[item.exercise.muscle] ?? 3.5) *
      3.5 *
      weightKg *
      activeSeconds) /
    (200 * 60);
  return Math.round(kcal * 10) / 10;
}
export function sessionCalories(
  session: Session,
  weightKg = CALORIE_REFERENCE_WEIGHT_KG,
) {
  const items = session.plan.items.map((item, index) => {
    const completedSets = session.actual[index].filter(
      (status) => status === "completed",
    ).length;
    return {
      exercise: item.exercise,
      completedSets,
      kcal: estimatedCalories(item, completedSets, weightKg),
    };
  });
  return {
    items,
    totalKcal: Math.round(items.reduce((sum, item) => sum + item.kcal, 0)),
  };
}
export const upsert = (history: Session[], session: Session) =>
  [session, ...history.filter((s) => s.id !== session.id)].sort((a, b) =>
    b.startedAt.localeCompare(a.startedAt),
  );

import {
  begin,
  counts,
  defaults,
  estimatedCalories,
  estimate,
  Exercise,
  finish,
  itemFor,
  mark,
  Plan,
  sessionCalories,
  upsert,
  validateConstraints,
  validateItem,
} from "./workout";

const exercise: Exercise = {
  id: "ex_test_push_up",
  name: "Push-up",
  muscle: "chest",
  equipment: ["bodyweight"],
  space: "small",
  environments: ["home", "indoor"],
  level: "beginner",
  timed: false,
  instructions: [],
};
const plan: Plan = {
  id: "plan-test",
  title: "Latihan manual",
  source: "manual",
  items: [itemFor(exercise, defaults)],
  createdAt: "2026-01-01T00:00:00.000Z",
};

describe("manual workout constraints", () => {
  test("duration boundaries and gym selection are validated", () => {
    expect(
      validateConstraints({ ...defaults, durationMinutes: 4 }),
    ).toBeTruthy();
    expect(validateConstraints({ ...defaults, durationMinutes: 90 })).toBe("");
    expect(
      validateConstraints({
        ...defaults,
        environment: "gym",
        gymId: undefined,
      }),
    ).toMatch(/Pilih gym/);
  });
  test("item parameter ranges and estimated duration are consistent", () => {
    const item = itemFor(exercise, defaults);
    expect(validateItem(item)).toBe(true);
    expect(validateItem({ ...item, sets: 0 })).toBe(false);
    expect(validateItem({ ...item, target: 101 })).toBe(false);
    expect(estimate([item])).toBeGreaterThan(0);
  });
});

describe("actual completion", () => {
  test("partial completion and pending sets remain explicit", () => {
    const planned = {
      ...plan,
      items: [{ ...itemFor(exercise), sets: 3, target: 12 }],
    };
    const started = begin(planned);
    const completed = finish(
      mark(mark(started, 0, 0, "completed"), 0, 1, "completed"),
    );
    expect(completed.plan.items[0].sets).toBe(3);
    expect(counts(completed)).toMatchObject({
      completed: 2,
      pending: 1,
      total: 3,
    });
    expect(completed.outcome).toBe("partial");
    expect(started.actual[0]).toEqual(["pending", "pending", "pending"]);
    expect(mark(completed, 0, 2, "completed")).toBe(completed);
  });
  test("skipped is not completed and saving is idempotent", () => {
    const completed = finish(mark(begin(plan), 0, 0, "skipped"));
    expect(counts(completed)).toMatchObject({
      completed: 0,
      skipped: 1,
      pending: 1,
    });
    expect(upsert(upsert([], completed), completed)).toHaveLength(1);
    expect(finish(completed)).toBe(completed);
  });
});

describe("local calorie estimate", () => {
  test("counts completed work only and totals estimates for a session", () => {
    const started = begin(plan);
    expect(estimatedCalories(plan.items[0], 0)).toBe(0);
    const partial = mark(started, 0, 0, "completed");
    const calories = sessionCalories(partial);
    expect(calories.items[0].kcal).toBe(estimatedCalories(plan.items[0], 1));
    expect(calories.totalKcal).toBeGreaterThan(0);
  });
});

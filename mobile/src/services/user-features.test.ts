import AsyncStorage from "@react-native-async-storage/async-storage";
import { featureStorage } from "./user-features";

beforeEach(async () => { await AsyncStorage.clear(); });

test("favorites are account-scoped and duplicate IDs are removed", async () => {
  await featureStorage.setFavorites("athlete@example.test", ["ex_push_up", "ex_push_up"]);
  await featureStorage.setFavorites("other@example.test", ["ex_plank"]);
  await expect(featureStorage.favorites("athlete@example.test")).resolves.toEqual(["ex_push_up"]);
  await expect(featureStorage.favorites("other@example.test")).resolves.toEqual(["ex_plank"]);
});

test("weekly goal defaults to three and accepts per-user values", async () => {
  await expect(featureStorage.weeklyGoal("athlete@example.test")).resolves.toBe(3);
  await featureStorage.setWeeklyGoal("athlete@example.test", 5);
  await expect(featureStorage.weeklyGoal("athlete@example.test")).resolves.toBe(5);
  await featureStorage.setWeeklyGoal("athlete@example.test", 12);
  await expect(featureStorage.weeklyGoal("athlete@example.test")).resolves.toBe(3);
});

test("saved preferences remain scoped to the signed-in account", async () => {
  const preset = {
    id: "preset-1",
    name: "Latihan di rumah",
    createdAt: "2026-11-25T00:00:00.000Z",
    constraints: {
      goal: "strength" as const,
      focus: "upper",
      experience: "beginner" as const,
      durationMinutes: 30,
      environment: "home" as const,
      space: "small" as const,
      equipment: ["bodyweight"],
      intensity: "moderate" as const,
    },
  };
  await featureStorage.setPresets("athlete@example.test", [preset]);
  await expect(featureStorage.presets("athlete@example.test")).resolves.toEqual([preset]);
  await expect(featureStorage.presets("other@example.test")).resolves.toEqual([]);
});

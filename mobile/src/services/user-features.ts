import AsyncStorage from "@react-native-async-storage/async-storage";
import { Constraints } from "../domain/workout";

export type WorkoutPreset = {
  id: string;
  name: string;
  constraints: Constraints;
  createdAt: string;
};

const key = (email: string, feature: string) =>
  `@ambatumove/${feature}/v1/${encodeURIComponent(email.trim().toLowerCase())}`;

export const featureStorage = {
  async favorites(email: string): Promise<string[]> {
    const raw = await AsyncStorage.getItem(key(email, "favorites"));
    return raw ? (JSON.parse(raw) as string[]) : [];
  },
  async setFavorites(email: string, ids: string[]) {
    await AsyncStorage.setItem(key(email, "favorites"), JSON.stringify([...new Set(ids)]));
  },
  async presets(email: string): Promise<WorkoutPreset[]> {
    const raw = await AsyncStorage.getItem(key(email, "presets"));
    return raw ? (JSON.parse(raw) as WorkoutPreset[]) : [];
  },
  async setPresets(email: string, values: WorkoutPreset[]) {
    await AsyncStorage.setItem(key(email, "presets"), JSON.stringify(values));
  },
  async weeklyGoal(email: string): Promise<number> {
    const raw = await AsyncStorage.getItem(key(email, "weekly-goal"));
    const value = raw ? Number(raw) : 3;
    return Number.isInteger(value) && value >= 1 && value <= 7 ? value : 3;
  },
  async setWeeklyGoal(email: string, value: number) {
    await AsyncStorage.setItem(key(email, "weekly-goal"), String(value));
  },
};

import AsyncStorage from "@react-native-async-storage/async-storage";
import { Session, upsert } from "../domain/workout";
export const DEMO = { email: "demo@ambatumove.app", password: "Move123!" };
export interface AuthService {
  login(email: string, password: string): Promise<string>;
}
export interface WorkoutHistoryRepository {
  list(): Promise<Session[]>;
  save(s: Session): Promise<Session[]>;
  remove(id: string): Promise<Session[]>;
}
const pause = () => new Promise((resolve) => setTimeout(resolve, 350));
export const mockAuth: AuthService = {
  async login(email, password) {
    await pause();
    if (email.trim().toLowerCase() !== DEMO.email || password !== DEMO.password)
      throw new Error("Email atau kata sandi demo tidak cocok.");
    return DEMO.email;
  },
};
export const keys = {
  auth: "@ambatumove/auth/v1",
  history: "@ambatumove/history/v1",
  draft: "@ambatumove/draft/v1",
  active: "@ambatumove/active/v1",
  catalog: "@ambatumove/catalog-cache/v1",
  equipment: "@ambatumove/equipment-cache/v1",
  gyms: "@ambatumove/gyms-cache/v1",
};
export async function read<T>(key: string, fallback: T): Promise<T> {
  const value = await AsyncStorage.getItem(key);
  return value ? (JSON.parse(value) as T) : fallback;
}
export const write = (key: string, value: unknown) =>
  AsyncStorage.setItem(key, JSON.stringify(value));
let saveQueue: Promise<unknown> = Promise.resolve();
export const localHistory: WorkoutHistoryRepository = {
  list: () => read<Session[]>(keys.history, []),
  save(session) {
    const job = saveQueue
      .catch(() => {})
      .then(async () => {
        const history = await localHistory.list();
        const updated = upsert(history, {
          ...session,
          savedAt: session.savedAt ?? new Date().toISOString(),
        });
        await write(keys.history, updated);
        return updated;
      });
    saveQueue = job;
    return job;
  },
  remove(id) {
    const job = saveQueue
      .catch(() => {})
      .then(async () => {
        const updated = (await localHistory.list()).filter(
          (session) => session.id !== id,
        );
        await write(keys.history, updated);
        return updated;
      });
    saveQueue = job;
    return job;
  },
};

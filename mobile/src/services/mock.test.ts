import AsyncStorage from "@react-native-async-storage/async-storage";
import { begin, Exercise, Plan, itemFor, finish } from "../domain/workout";
import { DEMO, keys, localHistory, mockAuth, read, write } from "./mock";

const exercise: Exercise = {
  id: "ex_test",
  name: "Push-up",
  muscle: "chest",
  equipment: ["bodyweight"],
  space: "small",
  environments: ["home"],
  level: "beginner",
  timed: false,
  instructions: [],
};
const plan: Plan = {
  id: "plan-test",
  title: "Sesi uji",
  source: "manual",
  items: [itemFor(exercise)],
  createdAt: "2026-01-01T00:00:00.000Z",
};
beforeEach(async () => {
  await AsyncStorage.clear();
});

test("history survives repository reload and duplicate saves", async () => {
  const session = finish(begin(plan));
  await Promise.all([localHistory.save(session), localHistory.save(session)]);
  const loaded = await localHistory.list();
  expect(loaded).toHaveLength(1);
  expect(loaded[0].actual).toEqual(session.actual);
  expect(loaded[0].savedAt).toBeTruthy();
});
test("failed history save rejects and retry works", async () => {
  const session = finish(begin(plan));
  jest.spyOn(AsyncStorage, "setItem").mockRejectedValueOnce(new Error("disk"));
  await expect(localHistory.save(session)).rejects.toThrow("disk");
  await localHistory.save(session);
  expect(await localHistory.list()).toHaveLength(1);
});
test("deleting a session persists the remaining history", async () => {
  const first = finish(begin(plan));
  const second = { ...finish(begin(plan)), id: "second-session" };
  await localHistory.save(first);
  await localHistory.save(second);
  const remaining = await localHistory.remove(first.id);
  expect(remaining.map((session) => session.id)).toEqual([second.id]);
});
test("malformed history storage surfaces an error without erasing data", async () => {
  await AsyncStorage.setItem(keys.history, "{broken");
  await expect(localHistory.list()).rejects.toThrow();
});
test("demo login validates credentials and persists only the account email", async () => {
  await expect(mockAuth.login(DEMO.email, "wrong")).rejects.toThrow();
  await expect(mockAuth.login(DEMO.email, DEMO.password)).resolves.toBe(
    DEMO.email,
  );
  await write(keys.auth, DEMO.email);
  expect(await read(keys.auth, null)).toBe(DEMO.email);
  expect(await AsyncStorage.getItem(keys.auth)).not.toContain(DEMO.password);
});

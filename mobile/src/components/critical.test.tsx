import { render, screen, fireEvent } from "@testing-library/react-native";
import { SessionSummary } from "./session-summary";
import Login from "../app/login";
import {
  begin,
  finish,
  mark,
  itemFor,
  Exercise,
  Plan,
} from "../domain/workout";
jest.mock("../hooks/store", () => ({ useStore: () => ({ login: jest.fn() }) }));
jest.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));
test("login validates an empty credential form without prefilled demo credentials", async () => {
  await render(<Login />);
  await fireEvent.press(screen.getByTestId("login"));
  expect(screen.getByText("Masukkan alamat email yang valid.")).toBeTruthy();
  expect(screen.getByTestId("email").props.value).toBe("");
});
test("summary renders actual partial completion and pending set honestly", async () => {
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
  const p: Plan = {
    id: "plan",
    title: "Uji workout",
    source: "manual",
    items: [{ ...itemFor(exercise), sets: 3, target: 12 }],
    createdAt: new Date().toISOString(),
  };
  const s = finish(mark(mark(begin(p), 0, 0, "completed"), 0, 1, "completed"));
  await render(<SessionSummary session={s} />);
  expect(screen.getByText("Sesi parsial")).toBeTruthy();
  expect(screen.getByText("Set 3: belum selesai")).toBeTruthy();
  expect(screen.getByText("Aktual: 2 set dikonfirmasi")).toBeTruthy();
});

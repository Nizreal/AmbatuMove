import { defaults, Exercise } from "../domain/workout";
import {
  API_CLIENT_BASES,
  ApiError,
  chatWithCoach,
  describeApiError,
  generateWorkout,
  getCurrentUser,
  getExercises,
  getGymEquipment,
  getGyms,
  getProgression,
  loginApi,
  mapExercise,
  resolveApiBase,
} from "./ambatumove";

const response = (payload: unknown, status = 200) =>
  ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
    text: async () => JSON.stringify(payload),
  }) as Response;

beforeEach(() => {
  jest.restoreAllMocks();
  jest.spyOn(console, "info").mockImplementation(() => {});
});

test("coach sends authenticated consent and bounded multi-turn context, then parses the backend answer", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(response({
    data: { answer: "Keep your wrists stacked under your shoulders.", safety_classification: "general", suggested_actions: ["Try a slower rep"], request_id: "req-coach-1", limitations: "General education only." },
    meta: { request_id: "req-coach-1" },
  }));
  const history = Array.from({ length: 8 }, (_, index) => ({ role: index % 2 ? "assistant" as const : "user" as const, content: `${index}:${"x".repeat(1100)}` }));
  await expect(chatWithCoach("secret-access-token", "  How do I do a push-up?  ", true, history)).resolves.toMatchObject({
    answer: "Keep your wrists stacked under your shoulders.",
    safety_classification: "general",
  });
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
  expect(url).toBe(`${API_CLIENT_BASES.ambatumove}/coach/chat`);
  expect(new Headers(init.headers).get("authorization")).toBe("Bearer secret-access-token");
  const sent = JSON.parse(String(init.body));
  expect(sent.message).toBe("How do I do a push-up?");
  expect(sent.consent_external_processing).toBe(true);
  expect(sent.previous_turns).toHaveLength(6);
  expect(sent.previous_turns.every((turn: { content: string }) => turn.content.length <= 1000)).toBe(true);
  expect(sent.include_profile_context).toBe(false);
  expect(sent.include_training_history).toBe(false);
});

test("catalog data keeps canonical exercise and equipment IDs", () => {
  const mapped = mapExercise({
    id: "ex_lat_pulldown",
    name: "Lat Pulldown",
    category: "strength",
    primary_muscles: ["back"],
    required_equipment: ["cable_machine"],
    difficulty: "beginner",
    measurement_type: "repetitions",
    environments: ["gym", "indoor"],
    minimum_space: "medium",
  });
  expect(mapped.id).toBe("ex_lat_pulldown");
  expect(mapped.equipment).toEqual(["cable_machine"]);
  expect(mapped.environments).toEqual(["gym", "indoor"]);
  expect(mapped.measurement).toBe("repetitions");
});

test("native development defaults to deployed HTTPS and web development uses its same-origin proxy", () => {
  expect(
    resolveApiBase(
      "android",
      true,
      undefined,
      "https://nizdri.stei.fun/api/v1",
      "/__dev_proxy/ambatumove",
    ),
  ).toBe("https://nizdri.stei.fun/api/v1");
  expect(
    resolveApiBase(
      "web",
      true,
      undefined,
      "https://nizdri.stei.fun/api/v1",
      "/__dev_proxy/ambatumove",
    ),
  ).toBe("/__dev_proxy/ambatumove");
  expect(
    resolveApiBase(
      "web",
      false,
      undefined,
      "https://nizdri.stei.fun/api/v1",
      "/__dev_proxy/ambatumove",
    ),
  ).toBe("https://nizdri.stei.fun/api/v1");
  expect(
    resolveApiBase(
      "android",
      true,
      "https://staging.example/api/v1/",
      "https://nizdri.stei.fun/api/v1",
      "/__dev_proxy/ambatumove",
    ),
  ).toBe("https://staging.example/api/v1");
});

test("workout generation saves an authenticated workout and maps API item IDs", async () => {
  const mapped: Exercise = mapExercise({
    id: "ex_bird_dog",
    name: "Bird Dog",
    category: "strength",
    primary_muscles: ["core"],
    required_equipment: ["bodyweight"],
    difficulty: "beginner",
    measurement_type: "repetitions",
    environments: ["home"],
    minimum_space: "small",
  });
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({
      data: {
        id: "workout_01",
        saved: true,
        version: 1,
        goal: "general_fitness",
        constraints: { environment: "home" },
        sections: [
          {
            type: "main",
            items: [
              {
                item_id: "wi_01",
                exercise_id: "ex_bird_dog",
                prescription: { sets: 3, reps: 10, rest_seconds: 45 },
              },
            ],
          },
        ],
        warnings: [],
      },
    }),
  );
  try {
    const plan = await generateWorkout(defaults, [mapped], "access-token");
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(url).toBe(`${API_CLIENT_BASES.ambatumove}/workouts/generate`);
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("content-type")).toBe(
      "application/json",
    );
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer access-token");
    expect(JSON.parse(String(init.body))).toMatchObject({
      save: true,
      available_equipment: ["bodyweight"],
    });
    expect(plan.items[0].exercise.id).toBe("ex_bird_dog");
    expect(plan.items[0]).toMatchObject({ sets: 3, target: 10, rest: 45, backendItemId: "wi_01" });
    expect(plan.backendWorkoutId).toBe("workout_01");
  } finally {
    fetchMock.mockRestore();
  }
});

test("gym preview sends the canonical gym ID and inventory mode", async () => {
  const exercise = mapExercise({
    id: "ex_lat_pulldown",
    name: "Lat Pulldown",
    category: "strength",
    primary_muscles: ["back"],
    required_equipment: ["cable_machine"],
    difficulty: "beginner",
    measurement_type: "repetitions",
    environments: ["gym", "indoor"],
    minimum_space: "medium",
  });
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({
      data: {
        id: "gym_workout_01",
        saved: true,
        version: 1,
        goal: "strength",
        constraints: { environment: "gym" },
        sections: [
          {
            type: "main",
            items: [
              {
                item_id: "wi_gym",
                exercise_id: "ex_lat_pulldown",
                prescription: { sets: 3, reps: 10 },
              },
            ],
          },
        ],
        warnings: [],
      },
    }),
  );
  try {
    await generateWorkout(
      {
        ...defaults,
        goal: "strength",
        environment: "gym",
        gymId: "gym_bandung_anytime_summarecon",
        equipment: ["barbell", "cable_machine"],
        equipmentSource: "gym_inventory",
      },
      [exercise],
      "access-token",
    );
    const [, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(JSON.parse(String(init.body))).toMatchObject({
      environment: "gym",
      gym_id: "gym_bandung_anytime_summarecon",
      equipment_source: "gym_inventory",
      available_equipment: [],
      save: true,
    });
  } finally {
    fetchMock.mockRestore();
  }
});

test("exercise catalog preserves pagination and requests the deployed API URL", async () => {
  const firstPage = Array.from({ length: 2 }, (_, i) => ({
    id: `ex_page_${i}`,
    name: `Exercise ${i}`,
    primary_muscles: ["back"],
    required_equipment: [],
  }));
  const secondPage = [
    {
      id: "ex_page_2",
      name: "Exercise 2",
      primary_muscles: ["back"],
      required_equipment: [],
    },
  ];
  const fetchMock = jest
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(
      response({
        data: firstPage,
        meta: { pagination: { limit: 2, offset: 0, has_more: true } },
      }),
    )
    .mockResolvedValueOnce(
      response({
        data: secondPage,
        meta: { pagination: { limit: 2, offset: 2, has_more: false } },
      }),
    );
  const exercises = await getExercises();
  expect(exercises.map((item) => item.id)).toEqual([
    "ex_page_0",
    "ex_page_1",
    "ex_page_2",
  ]);
  expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
    `${API_CLIENT_BASES.ambatumove}/exercises?limit=100&offset=0&sort=name`,
    `${API_CLIENT_BASES.ambatumove}/exercises?limit=100&offset=2&sort=name`,
  ]);
});

test("gym and gym inventory clients call the canonical endpoints", async () => {
  const fetchMock = jest
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(
      response({ data: [{ id: "gym_test", name: "Test Gym" }] }),
    )
    .mockResolvedValueOnce(
      response({
        data: [{ equipment_id: "cable_machine", label: "Cable Machine" }],
      }),
    );
  await expect(getGyms()).resolves.toMatchObject([
    { id: "gym_test", name: "Test Gym" },
  ]);
  await expect(getGymEquipment("gym_test")).resolves.toEqual([
    { id: "cable_machine", label: "Cable Machine" },
  ]);
  expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
    `${API_CLIENT_BASES.ambatumove}/gyms?limit=100&offset=0`,
    `${API_CLIENT_BASES.ambatumove}/gyms/gym_test/equipment`,
  ]);
});

test("concurrent gym inventory lookups share one request and cache its response", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({ data: [{ equipment_id: "dedupe_dumbbell", label: "Dumbbell" }] }),
  );
  const gymId = "gym_deduplication_test";
  const [first, second] = await Promise.all([
    getGymEquipment(gymId),
    getGymEquipment(gymId),
  ]);
  const cached = await getGymEquipment(gymId);
  expect(first).toEqual(second);
  expect(cached).toEqual(first);
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(fetchMock.mock.calls[0][0]).toBe(
    `${API_CLIENT_BASES.ambatumove}/gyms/${gymId}/equipment`,
  );
});

test("concurrent progression lookups for the same session share one request", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({ data: { lookback_days: 90, items: [] } }),
  );
  const [first, second] = await Promise.all([
    getProgression("progression-dedupe-token", "Asia/Jakarta"),
    getProgression("progression-dedupe-token", "Asia/Jakarta"),
  ]);
  expect(first).toEqual(second);
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(fetchMock.mock.calls[0][0]).toBe(
    `${API_CLIENT_BASES.ambatumove}/me/progression?days=90&limit=20&timezone=Asia%2FJakarta`,
  );
});

test("workout generation preserves gym constraints and reports an unfeasible backend response", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({ error: { code: "UNFEASIBLE_PLAN", message: "No compatible exercises." } }, 422),
  );
  const constraints = {
    ...defaults,
    goal: "strength" as const,
    focus: "upper",
    experience: "intermediate" as const,
    durationMinutes: 30,
    environment: "gym" as const,
    gymId: "gym_bandung_david_sports",
    space: "large" as const,
    equipment: ["dumbbell", "cable_machine"],
    equipmentSource: "gym_inventory" as const,
    intensity: "vigorous" as const,
  };
  await expect(
    generateWorkout(constraints, [], "access-token"),
  ).rejects.toMatchObject({ status: 422, code: "UNFEASIBLE_PLAN" });
  const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
  expect(JSON.parse(String(init.body))).toMatchObject({
    goal: "strength",
    workout_type: "strength",
    duration_minutes: 30,
    experience_level: "intermediate",
    environment: "gym",
    space: "large",
    available_equipment: [],
    equipment_source: "gym_inventory",
    gym_id: "gym_bandung_david_sports",
    focus_muscles: ["chest", "back", "shoulders", "biceps", "triceps"],
    intensity: "vigorous",
    save: true,
  });
});

test("login uses the deployed API and current-user request carries the bearer token", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({ data: {
      user: { id: "user-1", email: "athlete@example.test", role: "member" },
      access_token: "access-token",
      refresh_token: "refresh-token",
      token_type: "Bearer",
      access_token_expires_at: "2026-11-25T12:00:00Z",
      refresh_token_expires_at: "2026-12-25T12:00:00Z",
    }, meta: { request_id: "request-1" } }),
  );
  const pair = await loginApi("Athlete@example.test", "correct-horse-battery-staple");
  expect(pair.user.email).toBe("athlete@example.test");
  expect(fetchMock.mock.calls[0][0]).toBe(`${API_CLIENT_BASES.ambatumove}/auth/login`);
  expect(JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))).toEqual({ email: "athlete@example.test", password: "correct-horse-battery-staple" });
  fetchMock.mockResolvedValueOnce(response({ data: { id: "user-1", email: "athlete@example.test", role: "member" }, meta: { request_id: "request-2" } }));
  await expect(getCurrentUser("access-token")).resolves.toMatchObject({ id: "user-1", email: "athlete@example.test" });
  expect(new Headers((fetchMock.mock.calls[1][1] as RequestInit).headers).get("authorization")).toBe("Bearer access-token");
});

test("API errors preserve network, authentication, HTTP, and missing-data categories", () => {
  expect(
    describeApiError(new ApiError("network", "offline", "https://api.test")),
  ).toContain("https://api.test");
  expect(
    describeApiError(new ApiError("authentication", "denied", undefined, 401)),
  ).toContain("HTTP 401");
  expect(
    describeApiError(new ApiError("http", "upstream", undefined, 503)),
  ).toContain("HTTP 503");
  expect(
    describeApiError(new ApiError("missing-data", "not found", undefined, 404)),
  ).toContain("Data atau endpoint");
});

test("HTTP and missing catalog responses are classified without mock fallback", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch");
  fetchMock.mockResolvedValueOnce(
    response(
      { error: { code: "AUTH_REQUIRED", message: "Authentication required." } },
      401,
    ),
  );
  await expect(getGyms()).rejects.toMatchObject({
    kind: "authentication",
    status: 401,
    code: "AUTH_REQUIRED",
  });

  fetchMock.mockResolvedValueOnce(response({ data: [], meta: {} }));
  await expect(getExercises()).rejects.toMatchObject({
    kind: "missing-data",
  });

  fetchMock.mockResolvedValueOnce(
    response({ error: { code: "UPSTREAM_ERROR", message: "Try again." } }, 503),
  );
  await expect(getGyms()).rejects.toMatchObject({ kind: "http", status: 503 });

  fetchMock.mockRejectedValueOnce(new TypeError("Failed to fetch"));
  await expect(getGyms()).rejects.toMatchObject({ kind: "network" });
});

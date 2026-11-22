import { defaults, Exercise } from "../domain/workout";
import {
  API_CLIENT_BASES,
  ApiError,
  describeApiError,
  generateWorkout,
  getExercises,
  getGymEquipment,
  getGyms,
  getProfessionals,
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

test("workout generation sends a non-persisting request and maps API item IDs", async () => {
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
        id: null,
        saved: false,
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
    const plan = await generateWorkout(defaults, [mapped]);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(url).toBe(`${API_CLIENT_BASES.ambatumove}/workouts/generate`);
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("content-type")).toBe(
      "application/json",
    );
    expect(JSON.parse(String(init.body))).toMatchObject({
      save: false,
      available_equipment: ["bodyweight"],
    });
    expect(plan.items[0].exercise.id).toBe("ex_bird_dog");
    expect(plan.items[0]).toMatchObject({ sets: 3, target: 10, rest: 45 });
    expect(plan.backendWorkoutId).toBeNull();
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
        id: null,
        saved: false,
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
      save: false,
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

test("AmbatuFit directory uses the configured service client", async () => {
  const fetchMock = jest.spyOn(globalThis, "fetch").mockResolvedValue(
    response({
      items: [
        {
          id: "professional_1",
          display_name: "Demo Professional",
          specialty: "doctor",
          is_active: true,
        },
      ],
    }),
  );
  await expect(getProfessionals()).resolves.toEqual([
    {
      id: "professional_1",
      displayName: "Demo Professional",
      specialty: "doctor",
      description: undefined,
    },
  ]);
  expect(fetchMock.mock.calls[0][0]).toBe(
    `${API_CLIENT_BASES.ambatufit}/professionals`,
  );
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

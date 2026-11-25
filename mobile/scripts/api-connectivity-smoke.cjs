const { chromium } = require("playwright");

const origin = process.argv[2] || "http://localhost:8081";
const routes = [
  "/__dev_proxy/ambatumove/exercises?limit=100&offset=0&sort=name",
  "/__dev_proxy/ambatumove/equipment",
  "/__dev_proxy/ambatumove/gyms?limit=100&offset=0",
];

async function run() {
  const browser = await chromium.launch({ channel: "msedge", headless: true });
  try {
    const page = await browser.newPage();
    const requests = [];
    page.on("request", (request) => {
      if (request.url().includes("/__dev_proxy/")) {
        requests.push({ method: request.method(), url: request.url() });
      }
    });
    await page.goto(origin, { waitUntil: "domcontentloaded", timeout: 120000 });
    await page.getByTestId("login").waitFor({ timeout: 120000 });

    const catalogCalls = await page.evaluate(async (paths) => {
      const results = {};
      for (const path of paths) {
        const response = await fetch(path, {
          headers: { Accept: "application/json" },
        });
        const body = await response.json();
        results[path] = {
          status: response.status,
          count: Array.isArray(body.data)
            ? body.data.length
            : Array.isArray(body.items)
              ? body.items.length
              : null,
        };
      }
      return results;
    }, routes);

    const workoutPath = "/__dev_proxy/ambatumove/workouts/generate";
    const workout = await page.evaluate(async (path) => {
      const response = await fetch(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          goal: "general_fitness",
          workout_type: "mixed",
          duration_minutes: 20,
          experience_level: "beginner",
          environment: "home",
          space: "small",
          available_equipment: ["bodyweight"],
          equipment_source: "explicit",
          intensity: "moderate",
          save: false,
          seed: 20261123,
        }),
      });
      const body = await response.json();
      return {
        status: response.status,
        saved: body.data?.saved,
        itemCount: body.data?.sections?.reduce(
          (count, section) => count + (section.items?.length || 0),
          0,
        ),
      };
    }, workoutPath);

    const failures = Object.entries(catalogCalls).filter(
      ([, result]) => result.status !== 200 || result.count === null,
    );
    if (failures.length || workout.status !== 200 || workout.saved !== false) {
      throw new Error(
        `API smoke failed: ${JSON.stringify({ catalogCalls, workout })}`,
      );
    }
    if (requests.some((request) => request.method === "OPTIONS")) {
      throw new Error("Unexpected browser CORS preflight was sent.");
    }
    console.log(
      JSON.stringify(
        { origin, catalogCalls, workout, browserRequests: requests },
        null,
        2,
      ),
    );
  } finally {
    await browser.close();
  }
}

run().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});

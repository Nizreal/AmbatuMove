// Curated IDs only: exclude hidden bench / cable / bar dependencies in source data.
const fs = require("fs");
const path = require("path");
const source = require("../exercise-source.json");
const groups = {
  chest: [
    "Pushups",
    "Push-Up_Wide",
    "Clock_Push-Up",
    "Isometric_Chest_Squeezes",
    "Plyo_Push-up",
    "Pushups_Close_and_Wide_Hand_Positions",
  ],
  back: [
    "Bent_Over_Two-Dumbbell_Row",
    "Bent_Over_Two-Dumbbell_Row_With_Palms_In",
    "Superman",
    "Hyperextensions_With_No_Hyperextension_Bench",
    "Dumbbell_Shrug",
  ],
  biceps: [
    "Seated_Dumbbell_Curl",
    "Dumbbell_Bicep_Curl",
    "Dumbbell_Alternate_Bicep_Curl",
    "Alternate_Hammer_Curl",
    "Hammer_Curls",
    "Cross_Body_Hammer_Curl",
    "Standing_Dumbbell_Reverse_Curl",
    "Standing_Inner-Biceps_Curl",
    "Standing_Concentration_Curl",
  ],
  triceps: [
    "Dumbbell_Floor_Press",
    "Dumbbell_One-Arm_Triceps_Extension",
    "Standing_Dumbbell_Triceps_Extension",
    "Standing_Bent-Over_Two-Arm_Dumbbell_Triceps_Extension",
    "Push-Ups_-_Close_Triceps_Position",
    "Body-Up",
  ],
  shoulders: [
    "Seated_Dumbbell_Press",
    "Alternating_Deltoid_Raise",
    "Band_Pull_Apart",
    "Front_Dumbbell_Raise",
    "Front_Two-Dumbbell_Raise",
    "Side_Lateral_Raise",
    "Standing_Dumbbell_Press",
    "Dumbbell_Scaption",
    "Lateral_Raise_-_With_Bands",
    "Shoulder_Press_-_With_Bands",
  ],
  legs: [
    "Bodyweight_Squat",
    "Dumbbell_Squat",
    "Dumbbell_Lunges",
    "Dumbbell_Rear_Lunge",
    "Plie_Dumbbell_Squat",
    "Stiff-Legged_Dumbbell_Deadlift",
    "Standing_Dumbbell_Calf_Raise",
    "Butt_Lift_Bridge",
    "Single_Leg_Glute_Bridge",
    "Glute_Kickback",
    "Squats_-_With_Bands",
    "Band_Good_Morning",
  ],
  core: [
    "Dead_Bug",
    "Plank",
    "Side_Bridge",
    "Crunches",
    "Reverse_Crunch",
    "Alternate_Heel_Touchers",
    "Cross-Body_Crunch",
    "Russian_Twist",
    "Air_Bike",
    "Bent-Knee_Hip_Raise",
    "Cocoons",
    "Scissor_Kick",
    "Dumbbell_Side_Bend",
  ],
  cardio: [
    "Freehand_Jump_Squat",
    "Mountain_Climbers",
    "Star_Jump",
    "Knee_Tuck_Jump",
    "Standing_Long_Jump",
    "Lateral_Bound",
    "Inchworm",
  ],
};
const cues = {
  chest:
    "Jaga tubuh tetap stabil. Dorong dengan dada dan lengan tanpa mengunci siku.",
  back: "Jaga leher netral dan punggung stabil. Gerakkan bahu dengan kendali, tanpa menyentak.",
  biceps:
    "Dekatkan siku ke sisi tubuh. Tekuk siku untuk mengangkat beban, lalu turunkan perlahan.",
  triceps:
    "Stabilkan lengan atas. Luruskan siku dengan terkendali dan kembali perlahan.",
  shoulders:
    "Jaga rusuk tidak terangkat. Angkat lengan dengan kendali tanpa mengayun tubuh.",
  legs: "Jaga lutut searah jari kaki dan tumpuan stabil. Bergerak perlahan dalam rentang nyaman.",
  core: "Kencangkan perut, tetap bernapas. Hindari menarik leher atau melengkungkan pinggang.",
  cardio:
    "Mulai dengan tempo ringan. Jaga tumpuan stabil dan mendarat lembut jika melompat.",
};
async function main() {
  const rows = [];
  fs.mkdirSync(path.join(__dirname, "../assets/exercises"), {
    recursive: true,
  });
  for (const [muscle, ids] of Object.entries(groups))
    for (const id of ids) {
      const e = source.find((e) => e.id === id);
      if (!e) {
        console.log("Missing", id);
        continue;
      }
      const equipment =
        e.equipment === "dumbbell"
          ? ["dumbbell"]
          : e.equipment === "bands"
            ? ["band"]
            : [];
      if (id.startsWith("Seated_")) equipment.push("chair");
      const floor =
        ["core"].includes(muscle) ||
        /Bridge|Kickback|Superman|Hyperextensions|Floor_Press/.test(id);
      if (floor) equipment.push("mat");
      const jump = /Jump|Bound|Plyo/.test(id);
      const timed =
        [
          "Plank",
          "Side_Bridge",
          "Superman",
          "Isometric_Chest_Squeezes",
        ].includes(id) || muscle === "cardio";
      rows.push({
        id,
        name: e.name,
        muscle,
        equipment,
        space: /Long_Jump|Lateral_Bound/.test(id)
          ? "large"
          : muscle === "cardio"
            ? "medium"
            : "small",
        environments: ["indoor", "outdoor"],
        level: jump ? "advanced" : e.level === "expert" ? "advanced" : e.level,
        lowImpact: !jump,
        timed,
        instructions: [
          require("./exercise-cues.json")[id] || cues[muscle],
          "Gunakan rentang gerak yang nyaman. Hentikan gerakan jika terasa nyeri.",
        ],
        reference: `https://github.com/yuhonas/free-exercise-db/blob/main/exercises/${id}.json`,
        sourceInstructions: e.instructions,
      });
    }
  // Low-impact cardio with honest photo references (no unrelated image substitute).
  for (const [id, name, space, env] of [
    ["march", "Jalan di tempat", "small", ["indoor", "outdoor"]],
    ["walk", "Jalan cepat", "large", ["outdoor"]],
    ["step-touch", "Langkah ke samping", "medium", ["indoor", "outdoor"]],
  ])
    rows.push({
      id,
      name,
      muscle: "cardio",
      equipment: [],
      space,
      environments: env,
      level: "beginner",
      lowImpact: true,
      timed: true,
      instructions: [
        id === "march"
          ? "Angkat kaki bergantian sambil mengayun lengan. Tetap berdiri tegak."
          : id === "walk"
            ? "Berjalan dengan langkah nyaman, pandangan ke depan, dan ayunan lengan alami."
            : "Langkahkan kaki ke samping, rapatkan kaki lainnya, lalu ganti arah.",
        "Jaga tempo agar tetap dapat berbicara dengan nyaman.",
      ],
      reference: "https://www.nhs.uk/live-well/exercise/walking-for-health/",
    });
  let next = 0;
  await Promise.all(
    Array.from({ length: 6 }, async () => {
      while (next < rows.length) {
        const row = rows[next++];
        const e = source.find((e) => e.id === row.id);
        if (!e) continue;
        const dest = path.join(__dirname, `../assets/exercises/${row.id}.jpg`);
        if (!fs.existsSync(dest)) {
          const r = await fetch(
            `https://raw.githubusercontent.com/yuhonas/free-exercise-db/main/exercises/${e.images[0]}`,
          );
          if (!r.ok) throw new Error(row.id + ": " + r.status);
          fs.writeFileSync(dest, Buffer.from(await r.arrayBuffer()));
        }
        row.imageFile = true;
      }
    }),
  );
  fs.mkdirSync(path.join(__dirname, "../src/data"), { recursive: true });
  fs.writeFileSync(
    path.join(__dirname, "../src/data/catalog.ts"),
    `import { Exercise } from '../domain/workout';\nexport const catalog: Exercise[] = [\n${rows.map(({ imageFile, sourceInstructions, ...r }) => JSON.stringify(r).replace(/}$/, imageFile ? `,"image":require('../../assets/exercises/${r.id}.jpg')}` : "}")).join(",\n")}\n];\n`,
  );
  fs.writeFileSync(
    path.join(__dirname, "../src/data/source-instructions.json"),
    JSON.stringify(
      Object.fromEntries(
        rows
          .filter((r) => r.sourceInstructions)
          .map((r) => [r.id, r.sourceInstructions]),
      ),
      null,
      2,
    ),
  );
  console.log("Catalog:", rows.length);
}
main().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});

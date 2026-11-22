const { chromium } = require("playwright");
async function run() {
  const browser = await chromium.launch({ channel: "msedge", headless: true });
  try {
    const page = await browser.newPage({
      viewport: { width: 393, height: 852 },
      deviceScaleFactor: 1,
    });
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("console", (m) => {
      if (m.type() === "error") errors.push(m.text());
    });
    page.on("dialog", (dialog) => dialog.accept());
    const button = (name) => page.getByRole("button", { name, exact: true });
    await page.goto("http://localhost:8082", {
      waitUntil: "networkidle",
      timeout: 120000,
    });
    await page.getByTestId("login").waitFor({ timeout: 120000 });
    await page.getByTestId("login").click();
    await page
      .getByText("Masukkan alamat email yang valid.", { exact: true })
      .waitFor();
    await page.getByText("Isi akun demo", { exact: true }).click();
    await page.getByTestId("login").click();
    await page
      .getByText("Rekomendasi workout", { exact: true })
      .first()
      .waitFor();
    if (
      (await page.getByText("Riwayat workout", { exact: true }).count()) !== 0
    )
      throw new Error("Dashboard still exposes a history shortcut");
    await page.getByText("Riwayat", { exact: true }).click();
    await page.getByText("Belum ada sesi", { exact: true }).waitFor();
    await button("Rencanakan latihan").click();
    await button("Dapatkan rekomendasi").waitFor();
    await button("Kekuatan").click();
    await button("Biseps").click();
    await button("10 menit").click();
    await button("Dumbbell").click();
    await button("Dapatkan rekomendasi").click();
    await button("Mulai workout").waitFor();
    await button("Ganti gerakan").first().click();
    await button("Simpan perubahan").waitFor();
    await page.getByRole("radio").first().click();
    await button("Simpan perubahan").click();
    await button("Mulai workout").click();
    await button("Selesaikan set 1").click();
    await page.waitForFunction(
      () =>
        JSON.parse(localStorage.getItem("@ambatumove/active/v1"))
          .actual[0][0] === "completed",
    );
    await page.reload({ waitUntil: "networkidle" });
    await button("Selesaikan set 2").waitFor();
    await button("Lewati set tersisa gerakan ini").click();
    await button("Akhiri sesi").click();
    await button("Simpan hasil latihan").click();
    await button("Lihat riwayat").click();
    await page
      .getByRole("button", { name: /^Lihat / })
      .first()
      .click();
    await page.getByText("Sesi parsial", { exact: true }).waitFor();
    await page
      .getByText("Perkiraan kalori terbakar", { exact: true })
      .waitFor();
    await page.goto("http://localhost:8082");
    await page.getByText("Workout manual", { exact: true }).click();
    await button("Tambah dari katalog").click();
    await page
      .getByLabel("Cari gerakan atau otot", { exact: true })
      .fill("dead bug");
    await page.waitForFunction(
      () =>
        Array.from(document.querySelectorAll("img")).some(
          (img) => img.complete && img.naturalWidth > 0,
        ),
      null,
      { timeout: 10000 },
    );
    await button("Tambahkan gerakan").click();
    await page.getByLabel("Set (1–10)", { exact: true }).fill("3");
    await page.getByLabel("Nama latihan", { exact: true }).fill("Sesi manual");
    await button("Tambah dari katalog").click();
    await page
      .getByLabel("Cari gerakan atau otot", { exact: true })
      .fill("jalan di tempat");
    await button("Tambahkan gerakan").click();
    await button("Naik").last().click();
    await button("Hapus").first().click();
    await button("Mulai workout manual").click();
    await button("Selesaikan set 1").click();
    await button("Selesaikan set 2").click();
    await button("Akhiri sesi").click();
    await page.getByText("Set 3: belum selesai", { exact: true }).waitFor();
    await button("Simpan hasil latihan").click();
    await button("Lihat riwayat").click();
    await page
      .getByRole("button", { name: /^Lihat / })
      .first()
      .waitFor();
    await page.reload({ waitUntil: "networkidle" });
    await page
      .getByRole("button", { name: /^Lihat / })
      .first()
      .waitFor();
    const history = await page.evaluate(() =>
      JSON.parse(localStorage.getItem("@ambatumove/history/v1")),
    );
    if (
      history.length !== 2 ||
      history[0].actual[0].join(",") !== "completed,completed,pending"
    )
      throw new Error("Incorrect persisted history " + JSON.stringify(history));
    await page.getByRole("button", { name: /^Hapus Sesi manual$/ }).click();
    await page.waitForFunction(
      () =>
        JSON.parse(localStorage.getItem("@ambatumove/history/v1")).length === 1,
    );
    await page.getByText("Sesi manual", { exact: true }).waitFor({
      state: "detached",
    });
    await page.getByRole("tab", { name: "Profil" }).click();
    await page
      .getByText("demo@ambatumove.app", { exact: true })
      .first()
      .waitFor();
    await page.getByText("Keluar akun", { exact: true }).click();
    await page.getByTestId("login").waitFor();
    await page.goto("http://localhost:8082/manual/builder");
    await page.getByTestId("login").waitFor();
    await page.setViewportSize({ width: 320, height: 740 });
    console.log(
      "LOGIN, GUARD, RECOMMENDATION, SWAP, MANUAL, REORDER, REMOVE, PARTIAL, RELOAD, HISTORY, PROFILE, LOGOUT: PASSED",
    );
    if (errors.length)
      throw new Error("Browser console errors: " + JSON.stringify(errors));
  } finally {
    await browser.close();
  }
}
run().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});

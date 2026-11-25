const { chromium } = require("playwright");

async function run() {
  const browser = await chromium.launch({ channel: "msedge", headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 393, height: 852 }, deviceScaleFactor: 1 });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()); });
    await page.goto(process.argv[2] || "http://localhost:8082", { waitUntil: "domcontentloaded", timeout: 120000 });
    await page.getByTestId("login").waitFor({ timeout: 120000 });
    if ((await page.getByTestId("email").inputValue()) !== "") throw new Error("Login unexpectedly prefilled credentials");
    await page.getByTestId("login").click();
    await page.getByText("Masukkan alamat email yang valid.", { exact: true }).waitFor();

    await page.getByText("Belum punya akun? Buat akun", { exact: true }).click();
    await page.getByLabel("Email", { exact: true }).fill("athlete@example.test");
    await page.getByLabel("Kata sandi", { exact: true }).fill("short");
    await page.getByTestId("login").click();
    await page.getByText("Kata sandi harus terdiri dari minimal 12 karakter.", { exact: true }).waitFor();

    await page.setViewportSize({ width: 320, height: 740 });
    await page.screenshot({ path: ".validation-login.png", fullPage: true });
    if (errors.length) throw new Error("Browser console errors: " + JSON.stringify(errors));
    console.log("LOGIN VALIDATION, ACCOUNT CREATION VALIDATION, 320dp WEB PREVIEW: PASSED");
  } finally {
    await browser.close();
  }
}

run().catch((error) => { console.error(error); process.exitCode = 1; });

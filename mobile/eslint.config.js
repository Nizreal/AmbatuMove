const { defineConfig } = require("eslint/config");
const expo = require("eslint-config-expo/flat");
module.exports = defineConfig([
  expo,
  { ignores: ["dist/**", "coverage/**"] },
  {
    files: ["scripts/*.cjs"],
    languageOptions: { globals: { Buffer: "readonly", __dirname: "readonly" } },
  },
  {
    files: ["jest.setup.js"],
    languageOptions: { globals: { jest: "readonly" } },
  },
]);

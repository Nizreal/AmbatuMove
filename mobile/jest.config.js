module.exports = {
  preset: "jest-expo",
  testMatch: ["**/*.test.ts", "**/*.test.tsx"],
  setupFiles: ["./jest.setup.js"],
  testTimeout: 20000,
};

import swc from "unplugin-swc";
import { defineConfig } from "vitest/config";
import tsconfigPaths from "vite-tsconfig-paths";

import { createVitestTestConfig } from "./create-vitest-test-config";

export default defineConfig({
  plugins: [tsconfigPaths(), swc.vite()],
  test: {
    environment: "node",
    globals: true,
    include: ["src/**/*.spec.ts", "test/**/*.spec.ts"],
    coverage: {
      provider: "v8", // or 'istanbul'
      reportsDirectory: "./coverage",
    },
  },
});

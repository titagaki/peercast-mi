import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "base-path.spec.ts",
  use: { ...devices["Desktop Chrome"], baseURL: "http://127.0.0.1:4174" },
  webServer: {
    command:
      "PEERCAST_SITE_BASE_PATH=/mi npm run build && PEERCAST_SITE_BASE_PATH=/mi npm run preview -- --host 127.0.0.1 --port 4174 --strictPort",
    // Ignore the developer's local endpoint and test the production default.
    env: { VITE_PEERCAST_ENDPOINT: "" },
    wait: { stdout: /Local:.*4174/ },
    timeout: 30000,
  },
});

import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "base-path.spec.ts",
  use: { ...devices["Desktop Chrome"], baseURL: "http://127.0.0.1:4174" },
  webServer: {
    command:
      "PEERCAST_SITE_BASE_PATH=/mi npm run build && PEERCAST_SITE_BASE_PATH=/mi npm run preview -- --host 127.0.0.1 --port 4174 --strictPort",
    wait: { stdout: /Local:.*4174/ },
    timeout: 30000,
  },
});

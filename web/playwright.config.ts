import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  timeout: 45000,
  use: {
    baseURL: "http://127.0.0.1:18080",
    headless: true,
    viewport: { width: 1440, height: 1000 },
    trace: "retain-on-failure",
  },
  webServer: {
    command: "bash ../scripts/e2e-server.sh",
    url: "http://127.0.0.1:18080/healthz",
    reuseExistingServer: false,
    timeout: 30000,
  },
  reporter: "list",
});

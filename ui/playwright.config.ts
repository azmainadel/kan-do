import { defineConfig } from "@playwright/test";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Runs the real kan-do binary, with the UI built into it, on a fresh data folder.
const port = 4399;
const home = mkdtempSync(join(tmpdir(), "kando-e2e-"));
const bin = join(home, "kando");

export default defineConfig({
  testDir: "e2e",
  fullyParallel: false,
  workers: 1,
  use: { baseURL: `http://localhost:${port}`, trace: "retain-on-failure" },
  webServer: {
    command: `cd .. && go build -o ${bin} ./cmd/kando && ${bin} --home ${home} --no-open --port ${port}`,
    url: `http://localhost:${port}/api/ping`,
    timeout: 120_000,
  },
});

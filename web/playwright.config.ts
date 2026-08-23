import { defineConfig, devices } from "@playwright/test";
import { availableParallelism } from "node:os";

// ローカルではコアの4分の1を残し、共有 CI では半分だけを使用する。
function workerCount(): number {
  const cores = availableParallelism();
  if (process.env.CI) return Math.max(2, Math.floor(cores / 2));
  return Math.max(2, Math.floor((cores * 3) / 4));
}

// E2E では秘密鍵を表示する操作があるため、trace、動画、スクリーンショットを
// 保存しない。失敗は assertion とサーバーログから診断する。
export default defineConfig({
  testDir: "./e2e",
  // 各 test は専用の一時 HOME と、その中で起動した専用 sshc を持つ。
  // 同じ spec 内も並列化できるが、各 worker が Chromium と Go process を
  // 一つずつ動かす。
  fullyParallel: true,
  workers: workerCount(),
  // CI では不安定なテストを要約に記録するため一度だけ再試行する。
  retries: process.env.CI ? 1 : 0,
  forbidOnly: true,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: [["list"]],
  use: {
    ...devices["Desktop Chrome"],
    // このスイートは英語のテキストで要素を選ぶ。アプリケー
    // ションはブラウザから言語を選ぶため、ロケールをランナー任せ
    // にせずここで固定する。そうしなければ、同じ spec が実行
    // するマシンによって合格したり失敗したりしてしまう。
    locale: "en-US",
    trace: "off",
    video: "off",
    screenshot: "off",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] }, testIgnore: /narrow\.spec\.ts/ },
    // 360x800 の touch viewport では狭幅専用の導線だけを検証する。
    {
      name: "narrow",
      testMatch: /narrow\.spec\.ts/,
      use: { ...devices["Desktop Chrome"], viewport: { width: 360, height: 800 }, hasTouch: true },
    },
  ],
});

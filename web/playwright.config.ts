import { defineConfig, devices } from "@playwright/test";
import { availableParallelism } from "node:os";

// ローカルではコアの4分の1を残し、共有 CI では半分だけを使用する。
function workerCount(): number {
  const cores = availableParallelism();
  const requested = process.env.CI ? Math.floor(cores / 2) : Math.floor((cores * 3) / 4);
  // 各 worker は Chromium と sshc engine を1つずつ起動する。大きな開発機で
  // CPU 比率だけを使うと数十組が同時に ConPTY/PTY を確保し、製品の session
  // 上限や runner の process 起動時間を測るテストになってしまう。
  return Math.min(process.env.CI ? 8 : 12, Math.max(2, requested));
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
  // 失敗した操作は同じ run でやり直さない。flake も失敗として残し、traceを
  // 使わないこのsuiteでは assertion と engine logから原因を調べる。
  retries: 0,
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

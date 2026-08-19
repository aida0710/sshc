import { defineConfig, devices } from "@playwright/test";

// trace、動画、スクリーンショットはすべて意図的に無効化されてい
// る。1 つの end-to-end フローは設計上、秘密鍵を画面に表示するので
// あり、成果物のディレクトリはまさに secret が忘れ去られる類いの場
// 所だ。失敗は検証のメッセージとサーバー自身の出力から診断する。
export default defineConfig({
  testDir: "./e2e",
  // 各 test は専用の一時 HOME と、その中で起動した専用 sshc を持つ。
  // 同じ spec 内も並列化できるが、各 worker が Chromium と Go process を
  // 一つずつ動かすので、local は 4 に抑えて操作中の machine を塞がない。
  fullyParallel: true,
  workers: process.env.CI ? 2 : 4,
  // **CI でだけ、落ちたものをもう一度だけ走らせる。**
  //
  // 隠すためではない。**いま起きているのは、人が黙って再実行することである**
  // ——Windows の runner で 2 回続けて別々の test が落ち、どちらも変更なしの
  // 再実行で通った。その直し方だと、**不安定だったという事実がどこにも残らない。**
  // retries を入れると Playwright は "flaky" として数え、要約に出す——落ちた
  // ことも、二度目で通ったことも、両方見える。
  //
  // **1 回だけである。** 2 回 3 回と重ねると、本当に壊れているものが通って
  // しまう余地がそれだけ増える。手元では 0 のまま——直している最中に
  // 勝手に通られては困る。
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
    // 360x800 は、いま売られている最も狭い Android である。**ここで回すのは
    // narrow.spec.ts だけ** —— 既存の 17 本を狭い幅でもう一周させるには、
    // すべてをドロワー越しの操作へ書き換えることになり、守るものより書き
    // 換える量の方が多い。狭い幅でしか壊れないものだけを、そこで見る。
    //
    // hasTouch を立てるのは、hover に依存した導線がこの幅に残っていないことを
    // 同時に見るためである。
    {
      name: "narrow",
      testMatch: /narrow\.spec\.ts/,
      use: { ...devices["Desktop Chrome"], viewport: { width: 360, height: 800 }, hasTouch: true },
    },
  ],
});

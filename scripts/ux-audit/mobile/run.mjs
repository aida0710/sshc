// 使い方: node run.mjs <step.mjs> [nosnap]
// アプリの WebView に CDP でつなぎ、step の default(page) を実行する。
import { chromium } from "/home/aida/projects/sshc-worktrees/mobile-audit-1103/web/node_modules/playwright/index.mjs";
import { pathToFileURL } from "node:url";
const browser = await chromium.connectOverCDP("http://localhost:9223", { noDefaults: true });
const page = browser.contexts()[0].pages().find((p) => p.url().startsWith("http://127.0.0.1"));
page.on("console", (m) => { if (m.type() === "error" || m.type() === "warning") console.log("console:", m.type(), m.text()); });
page.on("pageerror", (e) => console.log("pageerror:", e.message));
const step = await import(pathToFileURL(process.argv[2]).href);
try { await step.default(page); } finally {
  console.log("URL", page.url());
  if (process.argv[3] !== "nosnap") console.log((await page.locator("body").ariaSnapshot()).split("\n").slice(0, 70).join("\n"));
  await browser.close();
}

// Windows の仮想マシンの Chrome（CDP をトンネルで r100 の 127.0.0.1:29222 へ）で sshc の画面を撮る（2026-11-03 の点検）。
// 使い方: node scripts/ux-audit/windows-capture.mjs <出力先> <sshc open の URL>
import { chromium } from "../../web/node_modules/playwright/index.mjs";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
const [out, openURL] = process.argv.slice(2);
mkdirSync(out, { recursive: true });
const masterPassword = "an audit master password for screenshots";
const browser = await chromium.connectOverCDP("http://127.0.0.1:29222", { timeout: 30000 });
const context = browser.contexts()[0];
const page = context.pages()[0] ?? await context.newPage();
await page.setViewportSize({ width: 1600, height: 900 });
const shot = async (name) => { await page.waitForTimeout(1200); await page.screenshot({ path: join(out, `${name}.png`) }); console.log("shot", name); };
const go = (path) => page.evaluate((target) => { window.history.pushState(null, "", target); window.dispatchEvent(new PopStateEvent("popstate")); }, path);
const step = async (label, action) => { try { await action(); } catch (error) { console.log("FAILED", label, String(error).split("\n")[0]); } };
await page.goto(openURL);
await page.waitForTimeout(3000);
await shot("01-entry");
const fields = page.locator('input[type="password"]');
if (await fields.first().isVisible().catch(() => false)) {
  for (let index = 0; index < await fields.count(); index++) await fields.nth(index).fill(masterPassword);
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(4000);
}
await shot("02-home");
for (const [name, path] of [["connections", "/connections"], ["menu", "/menu"], ["settings-general", "/settings/general"], ["settings-terminal", "/settings/terminal"], ["settings-appearance", "/settings/appearance"], ["keys", "/keys"], ["config", "/config"], ["files", "/files"]]) {
  await go(path);
  await shot(`03-${name}`);
}
await step("local shell", async () => {
  await page.getByRole("button", { name: "新しいセッション", exact: true }).first().click({ timeout: 8000 });
  await page.waitForTimeout(800);
  await shot("04-new-session");
  await page.getByRole("dialog").getByRole("button", { name: /^Local shell/ }).first().click({ timeout: 8000 });
  await page.waitForTimeout(5000);
  await page.locator(".xterm-screen").first().click();
  for (const line of ["Get-Location", "Get-ChildItem C:\\qa | Format-Table Name, Length", "echo 日本語の表示と全角の幅のテスト　｜漢字かなカナ"]) { await page.keyboard.type(line, { delay: 5 }); await page.keyboard.press("Enter"); await page.waitForTimeout(800); }
  await shot("05-local-powershell");
});
await step("ssh", async () => {
  await go("/connections");
  await page.waitForTimeout(1000);
  await page.getByRole("main").getByText("web-01", { exact: true }).first().click({ timeout: 8000 });
  await page.getByRole("button", { name: /^接続$/ }).first().click({ timeout: 8000 });
  await page.waitForTimeout(4000);
  await shot("06-ssh-host-key");
  await page.getByRole("button", { name: "接続する", exact: true }).first().click({ timeout: 8000 });
  await page.waitForTimeout(3000);
  await page.locator(".xterm-screen").first().click();
  await page.keyboard.type("fixture-password"); await page.keyboard.press("Enter");
  await page.waitForTimeout(3000);
  for (const line of ["ls -la --color=always", "printf '\\e[1;31m赤\\e[32m緑\\e[34m青\\e[0m 罫線 ┌─┬─┐ │あ│B│ └─┴─┘ 絵文字 😀👍\\n'"]) { await page.keyboard.type(line, { delay: 5 }); await page.keyboard.press("Enter"); await page.waitForTimeout(800); }
  await shot("07-ssh-session");
});
await browser.close();

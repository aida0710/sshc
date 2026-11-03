// アプリの全画面を開いて撮り、押す場所・切れた文字・はみ出し・名前の無いボタンを数える。結果は shots/<接頭辞>-routes.json。
import { writeFileSync } from "node:fs";
import { adb, tap, sleep, shot } from "./device.mjs";
import { measure } from "./measure.mjs";
const prefix = process.env.ROUTE_PREFIX ?? "std";
const only = process.env.ROUTE_ONLY ? new Set(process.env.ROUTE_ONLY.split(",")) : null;
const routes = [["home", "/"], ["menu", "/menu"], ["connections", "/connections"], ["terminal", "/terminal"], ["files", "/files"], ["snippets", "/snippets"], ["config", "/config"], ["keys", "/keys"], ["known-hosts", "/known-hosts"], ["remote-keys", "/install-key"], ["diagnostics", "/diagnostics"], ["vault-passwords", "/vault/passwords"], ["vault-key-passphrases", "/vault/key-passphrases"], ["vault-otp", "/vault/otp"], ["settings-general", "/settings/general"], ["settings-appearance", "/settings/appearance"], ["settings-terminal", "/settings/terminal"], ["settings-sftp", "/settings/sftp"], ["settings-shortcuts", "/settings/shortcuts"], ["settings-keybar", "/settings/keybar"], ["settings-notifications", "/settings/notifications"], ["settings-vault", "/settings/vault"], ["settings-machines", "/settings/machines"], ["settings-connections", "/settings/connections"], ["sync", "/sync"], ["vpn", "/vpn"], ["history", "/history"], ["license", "/license"], ["about", "/about"]];
export default async function (page) {
  const lock = page.locator('input[type="password"]').first();
  if (await lock.isVisible().catch(() => false)) { await lock.fill("phone-audit-pass"); await page.keyboard.press("Enter"); await sleep(3000); }
  const results = [];
  for (const [name, path] of routes.filter(([name]) => only === null || only.has(name))) {
    await page.evaluate((target) => { window.history.pushState(null, "", target); window.dispatchEvent(new PopStateEvent("popstate")); }, path);
    await sleep(1600);
    shot(`${prefix}-${name}`);
    const numbers = await page.evaluate(measure, true);
    results.push({ name, ...numbers });
    console.log(name, "small", numbers.smallTargets.length, "trunc", numbers.truncated.length, "overflowX", numbers.overflowX, "unnamed", numbers.unnamed, "tiny", numbers.tinyText.length);
  }
  writeFileSync(new URL(`../shots/${prefix}-routes.json`, import.meta.url), JSON.stringify(results, null, 1));
}

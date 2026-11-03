// 画面の移動（URL を変える）。下のバーを押す確認は別に行う。
import { sleep } from "./device.mjs";
export async function go(page, path) {
  await page.evaluate((target) => { window.history.pushState(null, "", target); window.dispatchEvent(new PopStateEvent("popstate")); }, path);
  await sleep(1500);
}

// tapCentered は、要素を画面の真ん中までスクロールしてから押す（下のバーの裏の要素を押さないため）。
import { tap } from "./device.mjs";
export async function tapCentered(page, locator) {
  await locator.evaluate((element) => element.scrollIntoView({ block: "center", inline: "center" }));
  await sleep(500);
  await tap(page, locator);
}

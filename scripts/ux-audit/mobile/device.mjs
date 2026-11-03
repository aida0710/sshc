// 実機の指の操作（adb input）を、ページの座標で送るための道具。
import { execFileSync } from "node:child_process";
const adbPath = "/home/aida/.local/share/android-sdk/platform-tools/adb";
export const adb = (...args) => execFileSync(adbPath, args, { encoding: "utf8" });
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// 縦向き・1080x2400・DPR 2.625 で測った値（ステータスバーの下からページが始まる）。向きを変えたら calibrate し直す。
let geometry = { dpr: 2.625, left: 0, top: 128 };
// ページの左上が画面のどこにあるかを、実際に1回タップして測る。
export async function calibrate(page) {
  // 画面の大きさ（wm size）を読み、右端の真ん中あたりを1回タップして、ページの左上が画面のどこかを測る。
  const size = adb("shell", "wm", "size");
  const match = (size.match(/Override size: (\d+)x(\d+)/) ?? size.match(/Physical size: (\d+)x(\d+)/));
  const [width, height] = [Number(match[1]), Number(match[2])];
  const [x, y] = [Math.round(width * 0.98), Math.round(height * 0.55)];
  await page.evaluate(() => { window.__probe = null; document.addEventListener("touchstart", (e) => { window.__probe = { x: e.touches[0].clientX, y: e.touches[0].clientY }; }, { once: true, capture: true }); });
  adb("shell", "input", "tap", String(x), String(y));
  await sleep(400);
  const probe = await page.evaluate(() => window.__probe);
  if (probe === null) throw new Error("the calibration tap did not reach the page");
  const dpr = await page.evaluate(() => devicePixelRatio);
  geometry = { dpr, left: x - probe.x * dpr, top: y - probe.y * dpr };
  return geometry;
}
export async function toDevice(page, x, y) {
  if (geometry === null) await calibrate(page);
  return [Math.round(geometry.left + x * geometry.dpr), Math.round(geometry.top + y * geometry.dpr)];
}
export async function tap(page, locator) {
  const box = await locator.boundingBox();
  if (box === null) throw new Error("not visible: " + locator);
  const [x, y] = await toDevice(page, box.x + box.width / 2, box.y + box.height / 2);
  adb("shell", "input", "tap", String(x), String(y));
}
export async function tapAt(page, cx, cy) {
  const [x, y] = await toDevice(page, cx, cy);
  adb("shell", "input", "tap", String(x), String(y));
}
export async function swipe(page, from, to, ms = 250) {
  const [x1, y1] = await toDevice(page, from[0], from[1]);
  const [x2, y2] = await toDevice(page, to[0], to[1]);
  adb("shell", "input", "swipe", String(x1), String(y1), String(x2), String(y2), String(ms));
}
export function shot(name) {
  const out = new URL(`../shots/${name}.png`, import.meta.url).pathname;
  const png = execFileSync(adbPath, ["exec-out", "screencap", "-p"], { maxBuffer: 64 * 1024 * 1024 });
  import("node:fs").then((fs) => fs.writeFileSync(out, png));
  return out;
}
export async function imeShown() {
  const out = adb("shell", "dumpsys", "input_method");
  return /mInputShown=true/.test(out);
}

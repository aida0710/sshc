// Claude Code の操作に使う道具: キーバーのキーを押す、空の送信（Enter）、入力欄から送る。
// fill はフォーカスを入れてキーボードを開くので、空の送信では使わない（入力欄が空のときだけ押す）。
import { tap, sleep, shot } from "./device.mjs";
const composer = (page) => page.getByPlaceholder("送る文を入力");
export async function key(page, name) {
  await tap(page, page.getByRole("button", { name, exact: true }).first());
  await sleep(800);
}
export async function enter(page) {
  if ((await composer(page).inputValue()) !== "") await composer(page).fill("");
  await tap(page, page.getByRole("button", { name: "送信", exact: true }).first());
  await sleep(1200);
}
export async function send(page, text) {
  await composer(page).fill(text);
  // fill で入力欄にフォーカスが入るとキーボードが開き、送信のボタンが上へ動く。動き終えてから押す。
  await sleep(1200);
  await tap(page, page.getByRole("button", { name: "送信", exact: true }).first());
  await sleep(300);
  if ((await composer(page).inputValue()) !== "") throw new Error("the composer still holds the text after send");
  await sleep(1500);
}
export { shot, sleep };

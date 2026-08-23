import type { Locator, Page } from "@playwright/test";

// ここは、**この検査群の中で `.xterm-*` を書いてよい唯一の場所**である。
//
// 製品側は既にそうなっている: `web/src/terminal/metrics.ts` が xterm の内部の
// 綴りを一手に持ち、`metrics.test.ts` が他所に漏れていないことを数える。
// **検査の側は、その後半が残っていた** ——2 つの spec に 20 箇所散っていた。
//
// **なぜそれが問題か。** `.xterm-rows` は DOM renderer が描く木の綴りである。
// WebGL renderer に替えると、字は canvas の中にあって DOM には無い。散っている
// 限り、renderer を替える試みは 20 箇所の書き換えから始まることになり、
// 「速いかどうか測ってみる」が安い実験でなくなる。
//
// 集めても、canvas に字が無いことは変わらない。**変わるのは、替えるときに
// 直す場所が一つになることである。**

const ROOT = ".xterm";
const DRAWN_ROWS = ".xterm-rows";
const SCREEN = ".xterm-screen";
const KEYBOARD = ".xterm-helper-textarea";
const SELECTION = ".xterm-selection div";
const VIEWPORT = ".xterm-viewport";

/** terminalRoot は、xterm が持っている一番外側の箱である。 */
export function terminalRoot(page: Page): Locator {
  return page.locator(ROOT);
}

/**
 * drawnRows は、xterm が描いた字が読める場所である。
 *
 * **section 全体を見ない。** あそこはキーバーを含むので、ボタンのラベルに
 * 一致してしまう。
 */
export function drawnRows(page: Page): Locator {
  return page.locator(DRAWN_ROWS);
}

/** drawnSpan は、描かれた字のうち text を含むものを返す。 */
export function drawnSpan(page: Page, text: string): Locator {
  return page.locator(`${DRAWN_ROWS} span`, { hasText: text });
}

/** terminalScreen は、字が描かれる面である。右クリックの的でもある。 */
export function terminalScreen(page: Page): Locator {
  return page.locator(SCREEN);
}

/**
 * terminalKeyboard は、打鍵が届く隠しテキストエリアである。
 *
 * **パネルのボタンでは代われない。** 打鍵はここへ入らなければ、端末は何も
 * 受け取らない。
 */
export function terminalKeyboard(page: Page): Locator {
  return page.locator(KEYBOARD);
}

/**
 * selectionMarks は、xterm が選択を表すために置く箱である。
 *
 * **ブラウザの選択ではない。** 選んだ範囲を知っているのは xterm だけである。
 */
export function selectionMarks(page: Page): Locator {
  return page.locator(SELECTION);
}

/** drawnRowCount は、いま描かれている行の本数を返す。 */
export function drawnRowCount(page: Page): Promise<number> {
  return page.locator(DRAWN_ROWS).evaluate((node) => node.children.length);
}

/**
 * drawnRowFont は、字を描いている場所の書体を返す。
 *
 * **字体を持っているのは行の箱である。** そこは xterm が桁を較正する場所でも
 * あり、metrics.ts が読んでいるのと同じ場所である。
 */
export function drawnRowFont(page: Page): Promise<{
  fontFamily: string;
  fontSize: string;
  letterSpacing: string;
}> {
  return page.locator(DRAWN_ROWS).evaluate((node) => {
    const style = getComputedStyle(node as HTMLElement);
    return {
      fontFamily: style.fontFamily,
      fontSize: style.fontSize,
      letterSpacing: style.letterSpacing,
    };
  });
}

/** screenRect は、字が描かれる面の位置と大きさを返す。 */
export function screenRect(page: Page): Promise<{ x: number; y: number; width: number; height: number }> {
  return page.locator(SCREEN).evaluate((node) => {
    const box = node.getBoundingClientRect();
    return { x: box.x, y: box.y, width: box.width, height: box.height };
  });
}

/** viewportBackground は、xterm が自分で塗っている面の色を返す。 */
export function viewportBackground(page: Page): Promise<string> {
  return page.locator(VIEWPORT).evaluate((node) => getComputedStyle(node as HTMLElement).backgroundColor);
}

/**
 * outsideTerminal は、その要素が xterm の木の外に在るかを答える。
 *
 * **これがあの設計そのものである。** xterm の中では長押しからの選択が
 * 始まらないので、選ばせたい字は必ずその外に無ければならない。
 */
export function outsideTerminal(node: Locator): Promise<boolean> {
  return node.evaluate((element, root) => element.closest(root) === null, ROOT);
}

/**
 * markTerminal は、作り直されたら消える印を、いまの端末に付ける。
 *
 * 返るのは、その印が付いたままの端末を指す locator である。
 */
export async function markTerminal(page: Page, mark: string): Promise<Locator> {
  await page.locator(ROOT).first().evaluate((node, value) => node.setAttribute("data-e2e-mark", value), mark);
  return page.locator(`${ROOT}[data-e2e-mark='${mark}']`);
}

/**
 * surfaceToken は、xterm を抱えている箱に効いている配色の値を読む。
 *
 * **xterm 自身ではなく親を読む。** 配色は箱の上の CSS 変数として置かれ、
 * xterm はそれを受け取って描く。**受け取ったことは、描いた字でしか分からない**
 * ので、これは「渡した側」の確認であり、drawnSpan が「受け取った側」である。
 */
export function surfaceToken(page: Page, name: string): Promise<string> {
  return page.locator(ROOT).evaluate((node, token) => {
    const box = (node as HTMLElement).parentElement!;
    return getComputedStyle(box).getPropertyValue(token).trim();
  }, name);
}

/** surfaceBackgroundImage は、その箱に敷かれた背景画像の指定を返す。 */
export function surfaceBackgroundImage(page: Page): Promise<string> {
  return page.locator(ROOT).evaluate((node) => {
    const box = (node as HTMLElement).parentElement!;
    return getComputedStyle(box).backgroundImage;
  });
}

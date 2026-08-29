import type { Locator, Page } from "@playwright/test";


const ROOT = ".xterm";
const DRAWN_ROWS = ".xterm-rows";
const SCREEN = ".xterm-screen";
const KEYBOARD = ".xterm-helper-textarea";
const SELECTION = ".xterm-selection div";
const VIEWPORT = ".xterm-viewport";
export function terminalRoot(page: Page): Locator {
  return page.locator(ROOT);
}
export function drawnRows(page: Page): Locator {
  return page.locator(DRAWN_ROWS);
}
export function drawnSpan(page: Page, text: string): Locator {
  return page.locator(`${DRAWN_ROWS} span`, { hasText: text });
}
export function terminalScreen(page: Page): Locator {
  return page.locator(SCREEN);
}
export function terminalKeyboard(page: Page): Locator {
  return page.locator(KEYBOARD);
}
export function selectionMarks(page: Page): Locator {
  return page.locator(SELECTION);
}
export function drawnRowCount(page: Page): Promise<number> {
  return page.locator(DRAWN_ROWS).evaluate((node) => node.children.length);
}
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
export function screenRect(page: Page): Promise<{ x: number; y: number; width: number; height: number }> {
  return page.locator(SCREEN).evaluate((node) => {
    const box = node.getBoundingClientRect();
    return { x: box.x, y: box.y, width: box.width, height: box.height };
  });
}
export function terminalFitRects(page: Page): Promise<{
  root: { x: number; y: number; width: number; height: number };
  host: { x: number; y: number; width: number; height: number };
}> {
  return page.locator(ROOT).evaluate((node) => {
    const rectangle = (element: Element) => {
      const box = element.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height };
    };
    return { root: rectangle(node), host: rectangle(node.parentElement!) };
  });
}
export function viewportBackground(page: Page): Promise<string> {
  return page.locator(VIEWPORT).evaluate((node) => getComputedStyle(node as HTMLElement).backgroundColor);
}
export function outsideTerminal(node: Locator): Promise<boolean> {
  return node.evaluate((element, root) => element.closest(root) === null, ROOT);
}
export async function markTerminal(page: Page, mark: string): Promise<Locator> {
  await page.locator(ROOT).first().evaluate((node, value) => node.setAttribute("data-e2e-mark", value), mark);
  return page.locator(`${ROOT}[data-e2e-mark='${mark}']`);
}
export function surfaceToken(page: Page, name: string): Promise<string> {
  return page.locator(ROOT).evaluate((node, token) => {
    const box = (node as HTMLElement).parentElement!;
    return getComputedStyle(box).getPropertyValue(token).trim();
  }, name);
}
export function surfaceBackgroundImage(page: Page): Promise<string> {
  return page.locator(ROOT).evaluate((node) => {
    const box = (node as HTMLElement).parentElement!;
    return getComputedStyle(box).backgroundImage;
  });
}

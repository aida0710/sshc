import type { Locator, Page } from "@playwright/test";


const ROOT = ".xterm";
const DRAWN_ROWS = ".xterm-rows";
const SCREEN = ".xterm-screen";
const KEYBOARD = ".xterm-helper-textarea";
const SELECTION = ".xterm-selection div";
const VIEWPORT = ".xterm-viewport";
const SCROLLBAR_SLIDER = ".xterm-scrollable-element > .xterm-scrollbar.xterm-vertical > .xterm-slider";
// These fit tests use 13px fonts: allow the 16px padding and one rounded row.
export const maxTerminalUnusedHeightPx = 40;
export async function waitForTerminalLayout(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await document.fonts.ready;
    // Font application, renderer layout and ResizeObserver's queued fit each
    // need a frame before the browser's final dimensions can be asserted.
    const layoutFrames = 3;
    for (let frame = 0; frame < layoutFrames; frame++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    }
  });
}
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
export function terminalCanvasCount(page: Page): Promise<number> {
  return page.locator(`${SCREEN} canvas`).count();
}
export function terminalKeyboard(page: Page): Locator {
  return page.locator(KEYBOARD);
}
export function terminalScrollbarSlider(page: Page): Locator {
  return page.locator(SCROLLBAR_SLIDER);
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
export function terminalDrawingRects(page: Page): Promise<{
  screen: { x: number; y: number; width: number; height: number };
  host: { x: number; y: number; width: number; height: number };
}> {
  return page.locator(SCREEN).evaluate((screen) => {
    const host = screen.closest("[data-terminal-host]");
    if (host === null) throw new Error("terminal screen has no host");
    const rectangle = (element: Element) => {
      const box = element.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height };
    };
    return { screen: rectangle(screen), host: rectangle(host) };
  });
}
export function loseTerminalWebGLContext(page: Page): Promise<boolean> {
  return page.locator(SCREEN).evaluate((screen) => {
    for (const canvas of screen.querySelectorAll("canvas")) {
      const context = canvas.getContext("webgl2") ?? canvas.getContext("webgl");
      const extension = context?.getExtension("WEBGL_lose_context");
      if (extension !== null && extension !== undefined) {
        extension.loseContext();
        return true;
      }
    }
    return false;
  });
}
export function terminalHostBottomOverflow(container: Locator): Promise<number> {
  return container.locator(ROOT).evaluate((node) => {
    const host = node.closest("[data-terminal-host]");
    if (host === null) return Number.POSITIVE_INFINITY;
    return node.getBoundingClientRect().bottom - host.getBoundingClientRect().bottom;
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

// Counts drawn pixels on the inline image layers (one above the text and, for
// kitty images placed under it, one below), so a missing or blank image reads
// as zero.
export function drawnImagePixels(page: Page): Promise<number> {
  return page.evaluate(() => {
    let drawn = 0;
    for (const layer of document.querySelectorAll<HTMLCanvasElement>("canvas[class*='xterm-image-layer']")) {
      const context = layer.getContext("2d");
      if (context === null || layer.width === 0 || layer.height === 0) continue;
      const pixels = context.getImageData(0, 0, layer.width, layer.height).data;
      for (let index = 3; index < pixels.length; index += 4) if ((pixels[index] ?? 0) > 0) drawn += 1;
    }
    return drawn;
  });
}

// The left edge in pixels of the first `character` on each drawn row, for the
// first `rows` rows that contain it. Only the DOM renderer draws rows as text.
export function drawnCharacterColumns(page: Page, character: string, rows: number): Promise<number[]> {
  return page.evaluate(({ character, rows }) => {
    const lefts: number[] = [];
    for (const row of document.querySelectorAll(".xterm-rows > div")) {
      const walker = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
        const at = node.textContent?.indexOf(character) ?? -1;
        if (at < 0) continue;
        const range = document.createRange();
        range.setStart(node, at);
        range.setEnd(node, at + character.length);
        lefts.push(Math.round(range.getBoundingClientRect().left));
        break;
      }
      if (lefts.length === rows) break;
    }
    return lefts;
  }, { character, rows });
}

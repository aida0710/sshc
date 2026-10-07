import { expect, type Page } from "@playwright/test";
import { windowsShell } from "./environment";
import { terminalKeyboard } from "./terminal";

type TerminalSize = { cols: number; rows: number };

export function watchTerminalStream(page: Page) {
  const sizes: TerminalSize[] = [];
  let output = "";
  let attachedStreams = 0;
  page.on("websocket", (socket) => {
    let received = false;
    socket.on("framesent", ({ payload }) => {
      try {
        const frame = JSON.parse(typeof payload === "string" ? payload : payload.toString("utf8")) as { resize?: TerminalSize };
        if (frame.resize !== undefined) sizes.push(frame.resize);
      } catch { /* Input frames are not resize messages. */ }
    });
    socket.on("framereceived", ({ payload }) => {
      if (!received) { received = true; attachedStreams++; }
      output += typeof payload === "string" ? payload : payload.toString("utf8");
    });
  });
  return { sizes, output: () => output, attachedStreams: () => attachedStreams };
}

export async function expectTerminalPTYSize(page: Page, observed: ReturnType<typeof watchTerminalStream>) {
  const size = observed.sizes.at(-1)!;
  expect(size.cols).toBeGreaterThan(1);
  expect(size.rows).toBeGreaterThan(1);
  const outputStart = observed.output().length;
  await terminalKeyboard(page).focus();
  await page.keyboard.type(windowsShell
    ? '"FIT_SIZE=$($Host.UI.RawUI.WindowSize.Height)-$($Host.UI.RawUI.WindowSize.Width)"'
    : 'printf "FIT_SIZE="; stty size | tr " " "-"');
  await page.keyboard.press("Enter");
  await expect.poll(() => observed.output().slice(outputStart)).toContain(`FIT_SIZE=${size.rows}-${size.cols}`);
}

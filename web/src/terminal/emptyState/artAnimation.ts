import { asciiFontFamily, beginFrame, createAsciiGrid, presentFrame, resolveAsciiColors } from "./asciiGrid";
import { followArtInput, type ArtInput } from "./artInput";
import { answersInput, createArt, framesPerSecond, type ArtOptions, type DrawnArt } from "./emptyTerminalArt";

export type ArtRun = {
  art: DrawnArt;
  fontSize: number;
  intensity: number;
  options: ArtOptions;
  // moving is false for one still frame (reduce motion, a sample in Settings).
  moving: boolean;
  // startAt is the moment of the art shown first, in seconds of the art's own clock.
  startAt: number;
};

// runArt draws an art on canvas and, when it moves, draws it again a few times a second while the page is shown.
// The art's clock runs only while it is drawn, so a page shown again goes on from where it stopped. It returns
// the function that stops it.
export function runArt(canvas: HTMLCanvasElement, run: ArtRun): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setInterval> | undefined;
  let stopInput = () => undefined as void;
  let start = () => undefined as void;
  const pause = () => {
    clearInterval(timer);
    timer = undefined;
  };
  const followVisibility = () => (document.hidden ? pause() : start());

  // The terminal's font may still be loading; drawing before it would measure the cells in a fallback font.
  void waitForFont(run.fontSize).then(() => {
    if (stopped) return;
    const grid = createAsciiGrid(canvas, run.fontSize, run.intensity, resolveAsciiColors());
    if (grid === null) return;
    const draw = createArt(run.art, grid, run.options);
    const input: ArtInput = { pointer: null, taps: [] };
    const frame = (seconds: number) => {
      beginFrame(grid);
      draw(seconds, input);
      presentFrame(grid);
    };
    if (!run.moving) {
      frame(run.startAt);
      return;
    }
    if (answersInput(run.art)) stopInput = followArtInput(canvas, grid, input);
    let shownSeconds = run.startAt;
    let lastTick = performance.now();
    const tick = () => {
      const now = performance.now();
      shownSeconds += (now - lastTick) / 1000;
      lastTick = now;
      frame(shownSeconds);
    };
    start = () => {
      if (timer !== undefined) return;
      lastTick = performance.now();
      tick();
      timer = setInterval(tick, 1000 / framesPerSecond[run.art]);
    };
    document.addEventListener("visibilitychange", followVisibility);
    if (!document.hidden) start();
  });

  return () => {
    stopped = true;
    pause();
    stopInput();
    document.removeEventListener("visibilitychange", followVisibility);
  };
}

function waitForFont(fontSize: number): Promise<unknown> {
  if (typeof document.fonts?.load !== "function") return Promise.resolve();
  return document.fonts.load(`${fontSize}px ${asciiFontFamily}`).catch(() => undefined);
}

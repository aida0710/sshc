import { describe, expect, it } from "vitest";
import { cellHeight, measureCells } from "./metrics";

// jsdom は要素に大きさを持たせない。**測る対象を偽るのではなく、測られる面を
// 偽る** ——getBoundingClientRect だけを差し替えれば、metrics は本物と同じ道を
// 通る。
function terminal(options: { screen?: DOMRect; rows: number; letterSpacing?: string }) {
  const element = document.createElement("div");
  element.className = "xterm";
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  const glyphs = document.createElement("div");
  glyphs.className = "xterm-rows";
  if (options.letterSpacing !== undefined) {
    glyphs.style.letterSpacing = options.letterSpacing;
    glyphs.style.fontFamily = "Menlo";
    glyphs.style.fontSize = "13px";
  }
  screen.appendChild(glyphs);
  element.appendChild(screen);
  document.body.appendChild(element);
  if (options.screen !== undefined) {
    const box = options.screen;
    screen.getBoundingClientRect = () => box;
  }
  return { element, rows: options.rows, screen };
}

function rect(width: number, height: number, left = 0, top = 0): DOMRect {
  return { width, height, left, top, right: left + width, bottom: top + height, x: left, y: top, toJSON: () => ({}) };
}

describe("measureCells", () => {
  it("divides the drawn surface by the number of rows", () => {
    const view = terminal({ screen: rect(800, 480), rows: 30 });
    expect(measureCells(view)?.cellHeight).toBe(16);
  });

  // **端数を捨てない。** 指で流す側は 1 行に満たない動きを溜めるので、丸めた
  // 高さを渡すと、ゆっくり引いた指がわずかにずれ続ける。
  it("keeps the fraction", () => {
    const view = terminal({ screen: rect(800, 521), rows: 30 });
    expect(measureCells(view)?.cellHeight).toBeCloseTo(17.3667, 3);
  });

  // **字送りを決めているのは family ではなく letter-spacing である。**
  it("copies the calibration the terminal resolved, not a guess", () => {
    const view = terminal({ screen: rect(800, 480), rows: 30, letterSpacing: "0.55px" });
    expect(measureCells(view)?.font.letterSpacing).toBe("0.55px");
    expect(measureCells(view)?.font.family).toBe("Menlo");
  });

  // **null と 0 は違う。** 呼ぶ側は「測れなかった」と「高さが 0 だった」を
  // 区別できなければならない。
  it("says nothing rather than zero while the surface is not up", () => {
    expect(measureCells({ element: undefined, rows: 30 })).toBeNull();
    expect(measureCells(terminal({ screen: rect(0, 0), rows: 30 }))).toBeNull();
    expect(measureCells(terminal({ screen: rect(800, 480), rows: 0 }))).toBeNull();
  });
});

describe("cellHeight", () => {
  // 面が建つ前に指が触れても死なない。
  it("falls back to the box until the surface is up", () => {
    const box = document.createElement("div");
    box.getBoundingClientRect = () => rect(800, 240);
    expect(cellHeight({ element: undefined, rows: 24 }, box)).toBe(10);
  });

  it("prefers the surface once it exists", () => {
    const box = document.createElement("div");
    box.getBoundingClientRect = () => rect(800, 999);
    expect(cellHeight(terminal({ screen: rect(800, 240), rows: 24 }), box)).toBe(10);
  });

  // **0 で割れば Infinity 行流れる。**
  it("refuses to divide by no rows", () => {
    const box = document.createElement("div");
    box.getBoundingClientRect = () => rect(800, 240);
    expect(cellHeight({ element: undefined, rows: 0 }, box)).toBe(0);
  });
});

// **xterm の DOM を知っているのは、このファイルの相手だけである。**
//
// 描画器を差し替えれば .xterm-screen も .xterm-rows も無くなる。掻く場所が
// 散らばっていると、そのとき直す先が分からない——実際、始める前は 2 か所が
// 別々に 1 行の高さを測っており、**導出まで食い違っていた**（片方は端数を
// 持ち、片方は丸めていた）。散文では守れないので、ここで数える。
describe("xterm の DOM を知る場所", () => {
  it("is metrics.ts and nowhere else", async () => {
    const { readdir, readFile } = await import("node:fs/promises");
    const { dirname, join } = await import("node:path");
    const { fileURLToPath } = await import("node:url");
    // **cwd に頼らない。** vitest をどこから起こしたかで答えが変わってはならない。
    const sourceRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

    async function sources(directory: string): Promise<string[]> {
      const entries = await readdir(directory, { withFileTypes: true });
      const found: string[] = [];
      for (const entry of entries) {
        const path = join(directory, entry.name);
        if (entry.isDirectory()) found.push(...(await sources(path)));
        else if (/\.tsx?$/.test(entry.name) && !entry.name.includes(".test.")) found.push(path);
      }
      return found;
    }

    // 散文で `.xterm-screen` と書くのは構わない。**縛るのは掻く呼び出しである。**
    const scrapes = /(?:querySelector|querySelectorAll|closest|matches|getElementsByClassName)\s*(?:<[^>]*>)?\s*\(\s*["'`][^"'`]*xterm/;
    const elsewhere: string[] = [];
    for (const path of await sources(sourceRoot)) {
      if (path.endsWith(join("terminal", "metrics.ts"))) continue;
      if (scrapes.test(await readFile(path, "utf8"))) elsewhere.push(path);
    }
    expect(elsewhere).toEqual([]);
  });
});

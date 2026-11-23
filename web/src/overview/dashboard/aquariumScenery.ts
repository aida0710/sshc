import { putCharacter, type AsciiColor, type AsciiGrid } from "./asciiGrid";
import { hash3 } from "./noise";

// The aquarium's scenery: the surface, the weed and the bubbles that rise from it, and the floor of sand and coral.
// The fish swim in front of it (aquariumArt.ts). The floor stays still; the rest follows from the time alone.

export type Weed = { column: number; height: number; phase: number };
type Fixture = { column: number; row: number; character: string; color: AsciiColor; opacity: number };

// The tank's rows: the surface is the top row, and the sand is the last two.
export const surfaceRow = 0;
export const sandRows = 2;

export type Scenery = { weeds: readonly Weed[]; floor: readonly Fixture[] };

export function laySceneryOut(grid: AsciiGrid, random: () => number): Scenery {
  const sandTop = grid.rows - sandRows;
  return { weeds: plantWeeds(grid, sandTop, random), floor: layFloor(grid, sandTop, random) };
}

// drawScenery puts the scenery as it is at seconds, behind the fish.
export function drawScenery(grid: AsciiGrid, scenery: Scenery, seconds: number): void {
  drawSurface(grid, seconds);
  drawBubbles(grid, scenery.weeds, seconds);
  for (const weed of scenery.weeds) drawWeed(grid, weed, seconds);
  for (const fixture of scenery.floor) putCharacter(grid, fixture.column, fixture.row, fixture.character, fixture.color, fixture.opacity);
}

// plantWeeds sets clumps of weed along the sand, each a few stalks of different heights.
function plantWeeds(grid: AsciiGrid, sandTop: number, random: () => number): Weed[] {
  // About one clump for this many columns. A stalk is at most this share of the water tall, and this many rows.
  const columnsPerClump = 28;
  const tallestShare = 0.4;
  const tallestRows = 14;
  const tallest = Math.max(2, Math.min(tallestRows, Math.round(sandTop * tallestShare)));
  const weeds: Weed[] = [];
  for (let column = 3 + Math.floor(random() * 10); column < grid.columns - 3; column += columnsPerClump + Math.floor(random() * 16) - 8) {
    const stalks = 2 + Math.floor(random() * 3);
    for (let stalk = 0; stalk < stalks; stalk++) {
      weeds.push({ column: column + stalk * 2, height: Math.max(2, Math.round(tallest * (0.45 + random() * 0.55))), phase: random() * Math.PI * 2 });
    }
  }
  return weeds;
}

// drawWeed draws a stalk of alternate brackets, swaying more toward its tip.
function drawWeed(grid: AsciiGrid, weed: Weed, seconds: number): void {
  const sandTop = grid.rows - sandRows;
  for (let step = 0; step < weed.height; step++) {
    const lean = step / weed.height;
    const sway = Math.sin(seconds * 0.9 + weed.phase + step * 0.45) * lean * 1.6;
    const character = (step + Math.round(weed.phase)) % 2 === 0 ? "(" : ")";
    putCharacter(grid, weed.column + Math.round(sway), sandTop - 1 - step, character, "green", 0.45 + lean * 0.3);
  }
}

// layFloor is the sand, with a coral and a starfish here and there.
function layFloor(grid: AsciiGrid, sandTop: number, random: () => number): Fixture[] {
  const fixtures: Fixture[] = [];
  const grains = ["_", ".", ",", "_", ":", "_", "'"];
  // The top row of the sand is sparser, so the floor has an uneven edge.
  const sparseTop = 0.55;
  for (let row = sandTop; row < grid.rows; row++) {
    for (let column = 0; column < grid.columns; column++) {
      const value = hash3(column, row, 11);
      if (row === sandTop && value > sparseTop) continue;
      fixtures.push({ column, row, character: grains[Math.floor(value * grains.length)] ?? "_", color: "yellow", opacity: row === sandTop ? 0.28 : 0.4 });
    }
  }
  const coral = ["\\|/", "\\\\|//"];
  for (let column = 6 + Math.floor(random() * 10); column < grid.columns - 6; column += 26 + Math.floor(random() * 22)) {
    const color: AsciiColor = random() < 0.5 ? "magenta" : "red";
    coral.forEach((row, offset) => {
      [...row].forEach((character, index) => fixtures.push({ column: column - offset + index, row: sandTop - 2 + offset, character, color, opacity: 0.55 }));
    });
    fixtures.push({ column: column + 4, row: sandTop - 1, character: "*", color: "yellow", opacity: 0.6 });
  }
  return fixtures;
}

// drawSurface draws the top row as two waves running against each other.
function drawSurface(grid: AsciiGrid, seconds: number): void {
  for (let column = 0; column < grid.columns; column++) {
    const wave = Math.sin(column * 0.45 + seconds * 1.3) + 0.5 * Math.sin(column * 0.17 - seconds * 0.7);
    putCharacter(grid, column, surfaceRow, wave > 0 ? "~" : "-", "cyan", 0.38);
  }
}

// drawBubbles lets bubbles rise from the foot of a few weeds, now and then and each at its own pace.
function drawBubbles(grid: AsciiGrid, weeds: readonly Weed[], seconds: number): void {
  // A source may let a bubble go every this many seconds, and does so this share of the time. A bubble rises
  // between these many rows a second. One stalk in this many lets bubbles go.
  const interval = 0.6;
  const released = 0.35;
  const slowest = 1.6;
  const fastest = 2.8;
  const stalksPerSource = 5;
  const start = grid.rows - sandRows - 1;
  const height = start - surfaceRow;
  weeds.forEach((weed, source) => {
    if (source % stalksPerSource !== 0) return;
    const oldest = Math.floor((seconds - height / slowest) / interval);
    for (let bubble = Math.max(0, oldest); bubble <= Math.floor(seconds / interval); bubble++) {
      if (hash3(source, bubble, 17) > released) continue;
      const age = seconds - bubble * interval;
      const rising = slowest + hash3(source, bubble, 23) * (fastest - slowest);
      const row = start - age * rising;
      if (row <= surfaceRow || age < 0) continue;
      const climbed = (start - row) / height;
      const column = weed.column + 1 + Math.round(Math.sin(age * 2.2 + bubble) * 1.1);
      putCharacter(grid, column, Math.round(row), climbed < 0.3 ? "." : climbed < 0.7 ? "o" : "O", "cyan", 0.5);
    }
  });
}

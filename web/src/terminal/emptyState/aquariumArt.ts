import { putCharacter, type AsciiColor, type AsciiGrid } from "./asciiGrid";
import { drawScenery, laySceneryOut, sandRows, surfaceRow } from "./aquariumScenery";
import type { ArtInput, GridCell } from "./artInput";
import { hash3, seededRandom } from "./noise";

// The aquarium: fish in the terminal's colours swim in front of waving weed (aquariumScenery.ts). Each fish is a
// connection used lately, larger the more it is used. The fish come to look at the mouse, and a tap drops food
// that they race to eat. Unlike the other arts the tank keeps where its fish are, so it is stepped frame by frame.

export type AquariumFish = { readonly label: string; readonly weight: number };
export type AquariumOptions = {
  // fish are the connections to swim. Fish that stand for no connection fill a tank with few.
  fish: readonly AquariumFish[];
  // labels writes each fish's name under it; without, a name shows only while the fish eats.
  labels: boolean;
  // swimDepth is how deep the fish swim at their ease, as a share of the water from the surface: on a phone's
  // banner the machine's line covers the lower part.
  swimDepth: number;
};

type FishSize = "small" | "medium" | "large";

// Each fish facing right; facing left is the mirror. The eye (°) is drawn in the ink, the rest in the fish's colour.
const sprites: Record<FishSize, readonly string[]> = {
  small: ["><>"],
  medium: ["><(((°>"],
  large: ["   _.-^-._ ", "><(((    °>", "   `-.v.-' "],
};
const mirrored: Record<string, string> = { "<": ">", ">": "<", "(": ")", ")": "(", "/": "\\", "\\": "/", "`": "'", "'": "`" };
const eye = "°";
const fishColors: readonly AsciiColor[] = ["yellow", "red", "cyan", "magenta", "blue"];
// The sizes of the fish that stand for no connection, which fill a tank with few connections.
const unnamedSizes: readonly FishSize[] = ["medium", "small", "small", "large", "small", "medium"];

// About one fish for this many cells of water, and so many fish at least and at most: fewer looks empty, more
// looks crowded.
const cellsPerFish = 900;
const fewestFish = 4;
const mostFish = 14;
// The two most used connections are large fish if they were opened in more than this many hours; the next three
// are medium. Water shallower than roomForLarge rows (a phone's banner) has no room for a large fish to pass another.
const largeFishCount = 2;
const mediumFishCount = 3;
const largeFishHours = 2;
const roomForLarge = 9;
// The fish swim at their ease at least this many rows deep, however shallow the share of the water.
const shallowestSwim = 4;

// Cells a second each size swims at its ease. Small fish dart; large ones cruise. A fish rises or sinks at most
// risePerSecond rows a second.
const cruiseSpeed: Record<FishSize, number> = { small: 4.2, medium: 2.6, large: 1.6 };
const risePerSecond = 1.5;
// At its ease a fish picks a new depth every few seconds (wanderFor plus up to wanderSpread), and turns back
// turnChance of the time.
const wanderFor = 3;
const wanderSpread = 6;
const turnChance = 0.2;
// A fish notices the mouse this near (in cells, a row counting as two columns), nears it at curiousSpeed of its
// cruise, and stops this far from it.
const curiosityReach = 22;
const curiousSpeed = 0.8;
const curiosityStop = 3;
// Food: pellets dropped by one tap, a row apart this much, sinking this many rows a second, lying on the sand this
// long. A fish this near eats one; it chases food this much faster than it cruises, and its name shows this long
// after it eats.
const pelletsPerTap = 3;
const pelletSpread = 0.6;
const foodSinking = 1.4;
const foodLasts = 12;
const foodReach = 1.2;
const chaseSpeedUp = 2.4;
const fedNameSeconds = 3;
// How strongly things are drawn: a fish and its eye, a name (brighter while its fish eats or is looked at), food.
const fishOpacity = 0.85;
const eyeOpacity = 0.9;
const nameOpacity = 0.32;
const noticedNameOpacity = 0.75;
const foodOpacity = 0.8;
// The longest step the tank takes between two frames: a page shown again goes on without fish jumping across.
const longestStep = 0.25;
// The same tank for the same grid, and each fish its own way of wandering.
const tankSeed = 20261109;
const fishSeedStep = 7919;
// The fish start spread over the depth by the golden ratio, not bunched by chance.
const goldenRatio = 0.618;

type SwimmingFish = {
  rows: readonly string[];
  mirroredRows: readonly string[];
  width: number;
  color: AsciiColor;
  label: string;
  size: FishSize;
  x: number;
  y: number;
  direction: 1 | -1;
  targetRow: number;
  wanderAt: number;
  fedAt: number;
  random: () => number;
};
type Pellet = { column: number; row: number; landedAt: number | null };
// Tank is where things may be: the fish swim at their ease down to swimFloor; food sinks to the sand.
type Tank = { grid: AsciiGrid; sandTop: number; swimFloor: number };
// Moment is the time a frame shows and how far it is from the frame before, in seconds.
type Moment = { seconds: number; step: number };

export function createAquariumArt(grid: AsciiGrid, options: AquariumOptions): (seconds: number, input: ArtInput) => void {
  const random = seededRandom(tankSeed + grid.columns * 31 + grid.rows);
  const sandTop = grid.rows - sandRows;
  const tank: Tank = { grid, sandTop, swimFloor: Math.max(shallowestSwim, Math.round(sandTop * options.swimDepth)) };
  const fish = stockTank(tank, options.fish, random);
  const scenery = laySceneryOut(grid, random);
  const pellets: Pellet[] = [];
  let last: number | null = null;

  return (seconds, input) => {
    const moment = { seconds, step: last === null ? 0 : Math.max(0, Math.min(longestStep, seconds - last)) };
    last = seconds;
    for (const tap of input.taps) dropFood(tank, pellets, tap);
    input.taps.length = 0;
    sinkFood(tank, pellets, moment);
    for (const one of fish) swim(tank, one, { moment, pointer: input.pointer, pellets });

    drawScenery(grid, scenery, seconds);
    for (const pellet of pellets) putCharacter(grid, Math.round(pellet.column), Math.round(pellet.row), pellet.landedAt === null ? "*" : ".", "yellow", foodOpacity);
    for (const one of fish) drawFish(grid, one, { seconds, labels: options.labels, pointer: input.pointer });
  };
}

// sizeOfRank is the size of the fish of the rank-th most used connection.
function sizeOfRank(rank: number, fish: readonly AquariumFish[]): FishSize {
  if (rank < largeFishCount && (fish[rank]?.weight ?? 0) > largeFishHours) return "large";
  if (rank < largeFishCount + mediumFishCount) return "medium";
  return "small";
}

function stockTank(tank: Tank, chosen: readonly AquariumFish[], random: () => number): SwimmingFish[] {
  const { grid, swimFloor } = tank;
  const count = Math.max(fewestFish, Math.min(mostFish, Math.round((grid.columns * swimFloor) / cellsPerFish)));
  const named = [...chosen].sort((a, b) => b.weight - a.weight).slice(0, count);
  const fitWater = (size: FishSize): FishSize => (size === "large" && swimFloor < roomForLarge ? "medium" : size);
  const stock = [
    ...named.map((one, rank) => ({ label: one.label, size: fitWater(sizeOfRank(rank, named)) })),
    ...Array.from({ length: count - named.length }, (_, index) => ({ label: "", size: fitWater(unnamedSizes[index % unnamedSizes.length] ?? "small") })),
  ];
  return stock.map(({ label, size }, index) => {
    const rows = sprites[size];
    const width = Math.max(...rows.map((row) => row.length));
    const padded = rows.map((row) => row.padEnd(width));
    const depth = Math.max(0, swimFloor - rows.length - 1);
    return {
      rows: padded,
      mirroredRows: padded.map((row) => [...row].reverse().map((character) => mirrored[character] ?? character).join("")),
      width,
      // Neighbours in the list get colours apart, with a little chance so that the pattern does not repeat.
      color: fishColors[(index * 3 + Math.floor(hash3(index, 7, 3) * 2)) % fishColors.length] ?? "yellow",
      label,
      size,
      x: random() * Math.max(1, grid.columns - width),
      y: surfaceRow + 1 + ((index * goldenRatio + random() * 0.2) % 1) * depth,
      direction: random() < 0.5 ? 1 : -1,
      targetRow: surfaceRow + 1 + random() * depth,
      wanderAt: random() * wanderFor,
      fedAt: -Infinity,
      random: seededRandom(tankSeed + index * fishSeedStep),
    };
  });
}

type Surroundings = { moment: Moment; pointer: GridCell | null; pellets: Pellet[] };

// swim moves a fish one frame on: to the nearest food if there is any, else to the mouse if it is near, else
// wherever it fancies.
function swim(tank: Tank, fish: SwimmingFish, { moment, pointer, pellets }: Surroundings): void {
  const { grid, swimFloor } = tank;
  const middleRow = Math.floor(fish.rows.length / 2);
  const bottom = Math.max(surfaceRow + 1, swimFloor - fish.rows.length);
  const mouthColumn = () => (fish.direction > 0 ? fish.x + fish.width - 1 : fish.x);
  let speed = cruiseSpeed[fish.size];

  const food = nearestPellet(pellets, { column: mouthColumn(), row: fish.y + middleRow });
  if (food !== null) {
    speed *= chaseSpeedUp;
    fish.direction = food.column >= fish.x + fish.width / 2 ? 1 : -1;
    fish.targetRow = food.row - middleRow;
    if (Math.abs(food.column - mouthColumn()) <= foodReach && Math.abs(food.row - (fish.y + middleRow)) <= foodReach) {
      pellets.splice(pellets.indexOf(food), 1);
      fish.fedAt = moment.seconds;
    }
  } else if (pointer !== null && cellDistance(pointer, { column: fish.x + fish.width / 2, row: fish.y + middleRow }) < curiosityReach) {
    const ahead = pointer.column - (fish.x + fish.width / 2);
    fish.direction = ahead >= 0 ? 1 : -1;
    fish.targetRow = pointer.row - middleRow;
    speed = Math.abs(ahead) < curiosityStop + fish.width / 2 ? 0 : speed * curiousSpeed;
  } else if (moment.seconds >= fish.wanderAt) {
    fish.targetRow = surfaceRow + 1 + fish.random() * (bottom - surfaceRow - 1);
    if (fish.random() < turnChance) fish.direction = fish.direction > 0 ? -1 : 1;
    fish.wanderAt = moment.seconds + wanderFor + fish.random() * wanderSpread;
  }

  fish.x += fish.direction * speed * moment.step;
  // A fish turns at the glass.
  if (fish.x + fish.width > grid.columns) {
    fish.x = grid.columns - fish.width;
    fish.direction = -1;
  } else if (fish.x < 0) {
    fish.x = 0;
    fish.direction = 1;
  }
  // Food may lie below where the fish swim at their ease, and they go down for it.
  const deepest = food === null ? bottom : tank.sandTop - fish.rows.length;
  const target = Math.max(surfaceRow + 1, Math.min(deepest, fish.targetRow));
  const most = risePerSecond * moment.step;
  fish.y += Math.max(-most, Math.min(most, target - fish.y));
}

type Point = { column: number; row: number };

// cellDistance is how far apart two points are in cells, a row counting as two columns since a cell is about
// twice as tall as wide.
function cellDistance(from: Point, to: Point): number {
  return Math.hypot(from.column - to.column, (from.row - to.row) * 2);
}

function nearestPellet(pellets: readonly Pellet[], from: Point): Pellet | null {
  let nearest: Pellet | null = null;
  let distance = Infinity;
  for (const pellet of pellets) {
    const away = cellDistance(pellet, from);
    if (away < distance) {
      nearest = pellet;
      distance = away;
    }
  }
  return nearest;
}

// dropFood lets a few pellets fall about where the tap was.
function dropFood(tank: Tank, pellets: Pellet[], tap: GridCell): void {
  for (let pellet = 0; pellet < pelletsPerTap; pellet++) {
    pellets.push({ column: tap.column + pellet - 1, row: Math.min(tank.sandTop - 1, tap.row - pellet * pelletSpread), landedAt: null });
  }
}

// sinkFood lets the pellets sink to the sand, and takes away those that have lain there long enough.
function sinkFood(tank: Tank, pellets: Pellet[], moment: Moment): void {
  const onSand = tank.sandTop - 1;
  for (let index = pellets.length - 1; index >= 0; index--) {
    const pellet = pellets[index];
    if (pellet === undefined) continue;
    if (pellet.landedAt === null) {
      pellet.row = Math.min(onSand, pellet.row + foodSinking * moment.step);
      if (pellet.row === onSand) pellet.landedAt = moment.seconds;
    } else if (moment.seconds - pellet.landedAt > foodLasts) {
      pellets.splice(index, 1);
    }
  }
}

type FishView = { seconds: number; labels: boolean; pointer: GridCell | null };

function drawFish(grid: AsciiGrid, fish: SwimmingFish, { seconds, labels, pointer }: FishView): void {
  const rows = fish.direction > 0 ? fish.rows : fish.mirroredRows;
  const left = Math.round(fish.x);
  const top = Math.round(fish.y);
  rows.forEach((row, offset) => {
    [...row].forEach((character, column) => {
      if (character === " ") return;
      putCharacter(grid, left + column, top + offset, character, character === eye ? "ink" : fish.color, character === eye ? eyeOpacity : fishOpacity);
    });
  });
  if (fish.label === "") return;
  const fed = seconds - fish.fedAt < fedNameSeconds;
  if (!labels && !fed) return;
  const watched = pointer !== null && cellDistance(pointer, { column: fish.x + fish.width / 2, row: fish.y }) < curiosityReach / 2;
  const opacity = fed || watched ? noticedNameOpacity : nameOpacity;
  const start = left + Math.round((fish.width - fish.label.length) / 2);
  [...fish.label].forEach((character, offset) => putCharacter(grid, start + offset, top + rows.length, character, "ink", opacity));
}

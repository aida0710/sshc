import { putCharacter, shadeCharacter, type AsciiGrid } from "./asciiGrid";
import { fractalNoise, hash3 } from "./noise";

// The globe: a turning planet drawn in the density of the characters, lit from the upper left, with arcs from
// where the viewer is to a few cities across the world, and runs along them. Stars twinkle around it.

// Where the globe sits, as shares of the canvas: its centre, and its radius against the shorter side. A centre
// below the canvas shows it rising like a horizon.
export type GlobePlacement = { readonly x: number; readonly y: number; readonly radius: number };

type Vector = readonly [number, number, number];

const axialTilt = (23.4 * Math.PI) / 180;
const light = { x: -0.45, y: 0.55, z: 0.7 };
// Radians the globe turns each second; it turns eastward, as the Earth does.
const turnPerSecond = 0.18;
// The latitude of the viewer's own point. The longitude comes from the time zone.
const homeLatitude = 35;
// Cities the arcs reach (latitude, longitude), spread across the world.
const cities: readonly (readonly [number, number])[] = [
  [1.35, 103.82], [50.11, 8.68], [38.9, -77.04], [45.52, -122.68], [-33.87, 151.21], [-23.55, -46.63],
];
const arcSamples = 48;
const runCharacters = ["#", "*", "+", "."];

const radians = (degrees: number) => (degrees * Math.PI) / 180;

// homeLongitude is the longitude of the viewer's time zone: 15 degrees an hour east of UTC.
function homeLongitude(): number {
  return -new Date().getTimezoneOffset() / 4;
}

export function createGlobeArt(grid: AsciiGrid, placement: GlobePlacement): (seconds: number) => void {
  const globe = {
    x: grid.width * placement.x, y: grid.height * placement.y,
    radius: Math.min(grid.width, grid.height) * placement.radius,
  };
  const home: readonly [number, number] = [homeLatitude, homeLongitude()];
  const surface = surfaceOf(grid, globe);
  return (seconds) => {
    const turn = -seconds * turnPerSecond + 2.1;
    drawSurface(grid, surface, turn, seconds);
    drawArcs(grid, globe, turn, home, seconds);
    for (const [index, [latitude, longitude]] of [home, ...cities].entries()) {
      const point = sphereToScreen(radians(latitude), radians(longitude), turn, globe, 1);
      if (point === null) continue;
      const pulse = 0.5 + 0.5 * Math.sin(seconds * 2.4 + index);
      putCharacter(grid, Math.floor(point.x / grid.cellWidth), Math.floor(point.y / grid.cellHeight), index === 0 ? "@" : "o", "accent", 0.7 + 0.3 * pulse);
    }
  };
}

type Globe = { x: number; y: number; radius: number };

// What each cell shows of the globe, worked out once for the grid: the planet only turns, so a cell keeps its
// latitude and lighting and only its longitude moves.
type Surface = {
  stars: { column: number; row: number; seed: number }[];
  rim: { column: number; row: number; brightness: number }[];
  cells: { column: number; row: number; latitude: number; longitude: number; brightness: number }[];
};

// The land, as noise over a grid of latitudes and longitudes, read by every cell of every frame. It is made the
// first time a globe is drawn, not when the page loads.
const landColumns = 360;
const landRows = 180;
let landMap: Float32Array | null = null;

function landMapOnce(): Float32Array {
  if (landMap !== null) return landMap;
  const map = new Float32Array(landColumns * landRows);
  for (let row = 0; row < landRows; row++) {
    const latitude = ((row + 0.5) / landRows - 0.5) * Math.PI;
    for (let column = 0; column < landColumns; column++) {
      const [x, y, z] = unitVector(latitude, (column / landColumns) * 2 * Math.PI);
      map[row * landColumns + column] = fractalNoise(x * 1.7 + 11, y * 1.7 + 11, z * 1.7 + 11, 4);
    }
  }
  landMap = map;
  return map;
}

function landAt(land: Float32Array, latitude: number, longitude: number): number {
  const row = Math.min(landRows - 1, Math.max(0, Math.floor((latitude / Math.PI + 0.5) * landRows)));
  const turns = longitude / (2 * Math.PI);
  const column = Math.floor((turns - Math.floor(turns)) * landColumns) % landColumns;
  return land[row * landColumns + column] ?? 0;
}

function surfaceOf(grid: AsciiGrid, globe: Globe): Surface {
  const surface: Surface = { stars: [], rim: [], cells: [] };
  for (let row = 0; row < grid.rows; row++) {
    for (let column = 0; column < grid.columns; column++) {
      const dx = ((column + 0.5) * grid.cellWidth - globe.x) / globe.radius;
      const up = -((row + 0.5) * grid.cellHeight - globe.y) / globe.radius;
      const distance = dx * dx + up * up;
      if (distance > 1) {
        const seed = hash3(column, row, 7);
        if (seed < starShare) surface.stars.push({ column, row, seed });
        continue;
      }
      const z = Math.sqrt(1 - distance);
      const brightness = 0.2 + 0.8 * Math.max(0, dx * light.x + up * light.y + z * light.z);
      if (distance > 0.955) {
        surface.rim.push({ column, row, brightness });
        continue;
      }
      // Back from the screen to the tilted planet.
      const tiltedY = up * Math.cos(axialTilt) + z * Math.sin(axialTilt);
      const tiltedZ = -up * Math.sin(axialTilt) + z * Math.cos(axialTilt);
      surface.cells.push({ column, row, latitude: Math.asin(Math.max(-1, Math.min(1, tiltedY))), longitude: Math.atan2(dx, tiltedZ), brightness });
    }
  }
  return surface;
}

function drawSurface(grid: AsciiGrid, surface: Surface, turn: number, seconds: number): void {
  const map = landMapOnce();
  for (const star of surface.stars) drawStar(grid, star.column, star.row, star.seed, seconds);
  for (const edge of surface.rim) putCharacter(grid, edge.column, edge.row, ".", "accent", 0.3 + 0.3 * edge.brightness);
  for (const cell of surface.cells) {
    const longitude = cell.longitude + turn;
    const land = landAt(map, cell.latitude, longitude);
    if (land > 0.52) {
      const landness = Math.min(1, (land - 0.52) / 0.12);
      putCharacter(grid, cell.column, cell.row, shadeCharacter(0.3 + 0.5 * cell.brightness * (0.6 + 0.4 * landness)), "ink", 0.1 + 0.32 * cell.brightness);
    } else if (onGraticule(cell.latitude, longitude)) {
      putCharacter(grid, cell.column, cell.row, "+", "accent", 0.1 + 0.3 * cell.brightness);
    } else {
      putCharacter(grid, cell.column, cell.row, shadeCharacter(0.1 + 0.2 * cell.brightness), "ink", 0.06 + 0.2 * cell.brightness);
    }
  }
}

// About one cell in a hundred around the globe holds a star, a third of them bright ones.
const starShare = 0.012;

function drawStar(grid: AsciiGrid, column: number, row: number, seed: number, seconds: number): void {
  putCharacter(grid, column, row, seed < starShare / 3 ? "*" : ".", "ink", 0.12 + 0.18 * (0.5 + 0.5 * Math.sin(seconds * 1.3 + seed * 900)));
}

// onGraticule is whether a point lies on a meridian (every 30 degrees) or a parallel (every 20 degrees).
function onGraticule(latitude: number, longitude: number): boolean {
  const meridian = Math.abs(((longitude / (Math.PI / 6)) % 1 + 1) % 1 - 0.5) > 0.47;
  const parallel = Math.abs(((latitude / (Math.PI / 9)) % 1 + 1) % 1 - 0.5) > 0.45;
  return meridian || parallel;
}

function drawArcs(grid: AsciiGrid, globe: Globe, turn: number, home: readonly [number, number], seconds: number): void {
  const from = unitVector(radians(home[0]), radians(home[1]));
  for (const [index, [latitude, longitude]] of cities.entries()) {
    const to = unitVector(radians(latitude), radians(longitude));
    const head = Math.floor(((seconds * 0.22 + index * 0.17) % 1) * arcSamples);
    for (let step = 1; step < arcSamples; step++) {
      const t = step / arcSamples;
      const [x, y, z] = slerp(from, to, t);
      // The arc rises off the surface towards its middle.
      const point = sphereToScreen(Math.asin(Math.max(-1, Math.min(1, y))), Math.atan2(z, x), turn, globe, 1 + 0.12 * Math.sin(Math.PI * t));
      if (point === null) continue;
      const column = Math.floor(point.x / grid.cellWidth), row = Math.floor(point.y / grid.cellHeight);
      const behind = head - step;
      if (behind >= 0 && behind < runCharacters.length) putCharacter(grid, column, row, runCharacters[behind] ?? ".", "accent", 0.95 - behind * 0.2);
      else putCharacter(grid, column, row, ".", "accent", 0.35);
    }
  }
}

// sphereToScreen puts a point on (or, with altitude above 1, above) the turning planet on the canvas, or gives
// null when the planet hides it.
function sphereToScreen(latitude: number, longitude: number, turn: number, globe: Globe, altitude: number): { x: number; y: number } | null {
  const viewLongitude = longitude - turn;
  const x = Math.cos(latitude) * Math.sin(viewLongitude);
  const tiltedY = Math.sin(latitude);
  const tiltedZ = Math.cos(latitude) * Math.cos(viewLongitude);
  const y = tiltedY * Math.cos(axialTilt) - tiltedZ * Math.sin(axialTilt);
  const z = tiltedY * Math.sin(axialTilt) + tiltedZ * Math.cos(axialTilt);
  if (z < -0.05 && Math.hypot(x, y) * altitude < 1) return null;
  return { x: globe.x + x * globe.radius * altitude, y: globe.y - y * globe.radius * altitude };
}

function unitVector(latitude: number, longitude: number): Vector {
  return [Math.cos(latitude) * Math.cos(longitude), Math.sin(latitude), Math.cos(latitude) * Math.sin(longitude)];
}

function slerp(from: Vector, to: Vector, t: number): Vector {
  const dot = Math.max(-1, Math.min(1, from[0] * to[0] + from[1] * to[1] + from[2] * to[2]));
  const angle = Math.acos(dot);
  if (angle < 1e-6) return from;
  const fromWeight = Math.sin((1 - t) * angle) / Math.sin(angle);
  const toWeight = Math.sin(t * angle) / Math.sin(angle);
  return [from[0] * fromWeight + to[0] * toWeight, from[1] * fromWeight + to[1] * toWeight, from[2] * fromWeight + to[2] * toWeight];
}

import { putCharacter, type AsciiGrid } from "./asciiGrid";
import { fractalNoise, seededRandom } from "./noise";

// The network: the connections drawn as points with their names, joined by lines that characters run along.
// A point lights up as a run reaches it. Behind them lies a faint dotted ground.

type Node = { column: number; row: number; label: string };
type Cell = { column: number; row: number; character: string };
type Edge = { from: number; to: number; cells: Cell[]; speed: number; phase: number; forward: boolean };

// The layout is the same each time for the same grid and names.
const layoutSeed = 20261108;
// About one point per this many cells, and points this far apart (CSS pixels) at most.
const cellsPerNode = 520;
const nodeSpacing = 150;
// The run along a line: its head, then a fading tail.
const runCharacters = ["#", "*", "+", "."];
// A point lights from this share of a run's way along the line.
const arrivingFrom = 0.88;

export function createNetworkArt(grid: AsciiGrid, labels: readonly string[]): (seconds: number) => void {
  const { nodes, edges } = layOut(grid, labels);
  const texture = textureOf(grid);
  return (seconds) => {
    for (const dot of texture) putCharacter(grid, dot.column, dot.row, dot.character, "ink", dot.opacity);
    const glow = nodes.map(() => 0);
    for (const edge of edges) {
      for (const cell of edge.cells) putCharacter(grid, cell.column, cell.row, cell.character, "ink", 0.16);
      const progress = (seconds * edge.speed + edge.phase) % 1;
      drawRun(grid, edge, progress);
      if (progress > arrivingFrom) {
        const reached = edge.forward ? edge.to : edge.from;
        glow[reached] = Math.max(glow[reached] ?? 0, (progress - arrivingFrom) / (1 - arrivingFrom));
      }
    }
    for (const [index, node] of nodes.entries()) {
      const light = glow[index] ?? 0;
      putCharacter(grid, node.column, node.row, light > 0.5 ? "@" : "o", "accent", 0.55 + light * 0.45);
      [...node.label].forEach((character, offset) => putCharacter(grid, node.column + 2 + offset, node.row, character, "ink", 0.28 + light * 0.3));
    }
  };
}

// textureOf is the faint dotted ground behind the network. It stays still: only the runs move, so a frame redraws
// few cells.
function textureOf(grid: AsciiGrid): { column: number; row: number; character: string; opacity: number }[] {
  const dots = [];
  for (let row = 0; row < grid.rows; row++) {
    for (let column = 0; column < grid.columns; column++) {
      const value = fractalNoise(column * 0.06, row * 0.12, 3.1, 3);
      if (value > 0.55) dots.push({ column, row, character: value > 0.66 ? ":" : ".", opacity: (value - 0.5) * 0.35 });
    }
  }
  return dots;
}

function drawRun(grid: AsciiGrid, edge: Edge, progress: number): void {
  const length = edge.cells.length;
  if (length === 0) return;
  const head = Math.floor(progress * length);
  runCharacters.forEach((character, step) => {
    const index = edge.forward ? head - step : length - 1 - head + step;
    const cell = edge.cells[index];
    if (cell !== undefined) putCharacter(grid, cell.column, cell.row, character, "accent", 0.9 - step * 0.2);
  });
}

function layOut(grid: AsciiGrid, labels: readonly string[]): { nodes: Node[]; edges: Edge[] } {
  const random = seededRandom(layoutSeed);
  const count = Math.max(5, Math.round((grid.columns * grid.rows) / cellsPerNode));
  const spacing = Math.min(nodeSpacing, Math.min(grid.width, grid.height) * 0.32);
  const nodes: Node[] = [];
  for (let attempt = 0; nodes.length < count && attempt < count * 40; attempt++) {
    const column = Math.floor(2 + random() * Math.max(1, grid.columns - 14));
    const row = Math.floor(1 + random() * Math.max(1, grid.rows - 2));
    const crowded = nodes.some((node) => Math.hypot((node.column - column) * grid.cellWidth, (node.row - row) * grid.cellHeight) < spacing);
    // Each name once: with fewer names than points, the rest are left unnamed rather than repeating a name.
    if (!crowded) nodes.push({ column, row, label: labels[nodes.length] ?? "" });
  }
  const edges: Edge[] = [];
  const joined = new Set<string>();
  for (const [index, node] of nodes.entries()) {
    const nearest = nodes
      .map((other, otherIndex) => ({ otherIndex, distance: Math.hypot((other.column - node.column) * grid.cellWidth, (other.row - node.row) * grid.cellHeight) }))
      .filter((candidate) => candidate.otherIndex !== index)
      .sort((a, b) => a.distance - b.distance)
      .slice(0, 2);
    for (const { otherIndex } of nearest) {
      const key = [index, otherIndex].sort((a, b) => a - b).join("-");
      const other = nodes[otherIndex];
      if (joined.has(key) || other === undefined) continue;
      joined.add(key);
      edges.push({ from: index, to: otherIndex, cells: lineCells(node, other), speed: 0.12 + random() * 0.18, phase: random(), forward: random() < 0.5 });
    }
  }
  return { nodes, edges };
}

// lineCells are the cells between two points (Bresenham's line), each with the character for the step into it:
// - across, | down, / or \ on a slant. The two end cells are the points themselves and are left out.
function lineCells(from: Node, to: Node): Cell[] {
  const cells: Cell[] = [];
  let x = from.column, y = from.row;
  const dx = Math.abs(to.column - x), dy = -Math.abs(to.row - y);
  const stepX = x < to.column ? 1 : -1, stepY = y < to.row ? 1 : -1;
  let error = dx + dy;
  while (x !== to.column || y !== to.row) {
    const doubled = 2 * error;
    let movedX = 0, movedY = 0;
    if (doubled >= dy) { error += dy; x += stepX; movedX = stepX; }
    if (doubled <= dx) { error += dx; y += stepY; movedY = stepY; }
    const character = movedY === 0 ? "-" : movedX === 0 ? "|" : movedX * movedY > 0 ? "\\" : "/";
    cells.push({ column: x, row: y, character });
  }
  return cells.slice(0, -1);
}

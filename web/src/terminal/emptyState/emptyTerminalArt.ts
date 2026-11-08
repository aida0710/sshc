import type { SettingsPreset } from "../../settings/settingsStore";
import type { AsciiGrid } from "./asciiGrid";
import { createContoursArt } from "./contoursArt";
import { createGlobeArt, type GlobePlacement } from "./globeArt";
import { createNetworkArt } from "./networkArt";

// EmptyTerminalArt is what the terminal draws behind its empty state: nothing, or one of the moving ASCII arts.
// The preset keeps it as appearance.emptyTerminalArt; the engine checks the name against the same list.
export type EmptyTerminalArt = "none" | "network" | "contours" | "globe";

export const emptyTerminalArts: readonly EmptyTerminalArt[] = ["none", "network", "contours", "globe"];

// Nothing is drawn until the art is chosen: a moving picture costs battery, and the empty state looked so before.
export const defaultEmptyTerminalArt: EmptyTerminalArt = "none";

export type DrawnArt = Exclude<EmptyTerminalArt, "none">;

// Frames a second for each art. The runs along the network's lines move a cell at a time and want the most; the
// globe turns and the landscape drifts by a fraction of a cell a frame, so fewer frames look the same and cost less.
export const framesPerSecond: Record<DrawnArt, number> = { network: 12, contours: 4, globe: 6 };

export function isEmptyTerminalArt(value: unknown): value is EmptyTerminalArt {
  return typeof value === "string" && (emptyTerminalArts as readonly string[]).includes(value);
}

export function emptyTerminalArtOf(preset: SettingsPreset): EmptyTerminalArt {
  const stored = preset.appearance?.emptyTerminalArt;
  return isEmptyTerminalArt(stored) ? stored : defaultEmptyTerminalArt;
}

// withEmptyTerminalArt is the preset keeping art. The default is kept as no value.
export function withEmptyTerminalArt(preset: SettingsPreset, art: EmptyTerminalArt): SettingsPreset {
  const appearance = { ...preset.appearance };
  delete appearance.emptyTerminalArt;
  if (art !== defaultEmptyTerminalArt) appearance.emptyTerminalArt = art;
  return { ...preset, appearance };
}

export type ArtOptions = {
  // labels are the names the network writes beside its points: the connections recently used.
  labels: readonly string[];
  globe: GlobePlacement;
  // zoom shrinks the landscape of the contours, for a small canvas.
  zoom: number;
};

export function createArt(art: DrawnArt, grid: AsciiGrid, options: ArtOptions): (seconds: number) => void {
  switch (art) {
    case "network":
      return createNetworkArt(grid, options.labels);
    case "contours":
      return createContoursArt(grid, options.zoom);
    case "globe":
      return createGlobeArt(grid, options.globe);
  }
}

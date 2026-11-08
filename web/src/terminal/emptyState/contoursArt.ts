import { putCharacter, shadeCharacter, type AsciiGrid } from "./asciiGrid";
import { fractalNoise, hash3 } from "./noise";

// Contours: a slowly drifting landscape drawn in the density of the characters, like halftone dots, with the
// contour lines of the high ground in the accent colour.

// Contour lines every 1/contourSteps of height, only above contourFrom, and as wide as contourWidth of a step.
const contourSteps = 8;
const contourFrom = 0.5;
const contourWidth = 0.045;
// Ground lower than groundFrom is left as sparse dots; groundRange of height above it goes from light to dark.
const groundFrom = 0.46;
const groundRange = 0.32;

// Three octaves of noise: a fourth adds detail finer than a character cell, and costs a quarter more each frame.
const landscapeOctaves = 3;

// zoom shrinks the landscape: a small canvas (a banner, a sample in Settings) shows more of it, or it has no relief.
export function createContoursArt(grid: AsciiGrid, zoom: number): (seconds: number) => void {
  return (seconds) => {
    for (let row = 0; row < grid.rows; row++) {
      for (let column = 0; column < grid.columns; column++) {
        const height = fractalNoise(column * 0.03 * zoom + seconds * 0.04, row * 0.06 * zoom, seconds * 0.03, landscapeOctaves);
        const band = height * contourSteps;
        if (height > contourFrom && Math.abs(band - Math.round(band)) < contourWidth) {
          putCharacter(grid, column, row, "~", "accent", 0.22 + (height - contourFrom) * 1.3);
          continue;
        }
        const shade = Math.max(0, Math.min(1, (height - groundFrom) / groundRange));
        if (shade > 0) putCharacter(grid, column, row, shadeCharacter(0.1 + shade * 0.85), "ink", 0.05 + shade * 0.26);
        else if (hash3(column, row, 5) < 0.18) putCharacter(grid, column, row, ".", "ink", 0.05);
      }
    }
  };
}

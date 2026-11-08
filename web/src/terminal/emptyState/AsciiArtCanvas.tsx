import { useEffect, useRef } from "react";
import { useAccent } from "../../theme/accentContext";
import { useTheme } from "../../theme/context";
import { useReducedMotion } from "../../theme/motionContext";
import { runArt } from "./artAnimation";
import type { ArtOptions, DrawnArt } from "./emptyTerminalArt";
import { useElementSize } from "./useElementSize";

// A moment of the art shown when it does not move: one where the runs are on their way and the globe shows land.
const stillSeconds = 6;

// AsciiArtCanvas draws one of the ASCII arts filling its box. It moves while the page is shown and motion is not
// reduced; otherwise it shows one still frame. It is decoration, so screen readers skip it.
export function AsciiArtCanvas({
  art,
  fontSize,
  options,
  intensity = 1,
  still = false,
  stillAt = stillSeconds,
  className = "",
}: {
  art: DrawnArt;
  fontSize: number;
  options: ArtOptions;
  intensity?: number;
  still?: boolean;
  stillAt?: number;
  className?: string;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const size = useElementSize(canvas);
  const reducedMotion = useReducedMotion();
  const moving = !still && !reducedMotion;
  // The colours are read from the page, so the art is drawn again when the theme or the accent changes.
  const { resolved: theme } = useTheme();
  const { accent, customColor } = useAccent();
  // The options are compared by value: a parent that builds them on every render does not restart the art.
  const labelsKey = options.labels.join("\n");
  const { x: globeX, y: globeY, radius: globeRadius } = options.globe;
  const { zoom } = options;

  useEffect(() => {
    if (canvas.current === null || size === null) return;
    return runArt(canvas.current, {
      art, fontSize, intensity, moving, startAt: stillAt,
      options: { labels: labelsKey === "" ? [] : labelsKey.split("\n"), globe: { x: globeX, y: globeY, radius: globeRadius }, zoom },
    });
  }, [art, fontSize, intensity, moving, stillAt, size, theme, accent, customColor, labelsKey, globeX, globeY, globeRadius, zoom]);

  return <canvas ref={canvas} aria-hidden="true" className={`block h-full w-full ${className}`} />;
}

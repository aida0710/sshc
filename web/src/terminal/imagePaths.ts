import type { IBuffer } from "@xterm/xterm";
import { findBufferLinks } from "./bufferLinks";
import { resolveTerminalPath } from "./relativePaths";

// Names that say "image". Stepping through the preview should pass log lines
// such as /usr/bin/env by; the engine still checks the bytes of each file it reads.
const imageNamePattern = /\.(?:png|jpe?g|gif|webp|bmp)$/iu;

// imagePathsInBuffer lists the image paths in a terminal's output, oldest
// first and each once, so the preview can step from one to the next. Relative
// paths use directory and ~/ paths use home; unknown bases are left out.
export function imagePathsInBuffer(buffer: IBuffer, directory: string | null, home: string | null): string[] {
  const resolved = imagePathTargetsInBuffer(buffer)
    .map((path) => resolveTerminalPath(path, directory, home))
    .filter((path) => path !== null);
  return [...new Set(resolved)];
}

// Keep the printed targets available so opening an ordinary path does not
// need a home lookup when there are no home-relative images to navigate to.
export function imagePathTargetsInBuffer(buffer: IBuffer): string[] {
  const paths = new Set<string>();
  // findBufferLinks returns the links that touch one row, so a path that wraps
  // is found from each of its rows; the set keeps it once.
  for (let row = 1; row <= buffer.length; row += 1) {
    for (const { match } of findBufferLinks(buffer, row)) {
      if (match.kind === "url" || !imageNamePattern.test(match.target)) continue;
      paths.add(match.target);
    }
  }
  return [...paths];
}

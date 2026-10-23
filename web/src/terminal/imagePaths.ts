import type { IBuffer } from "@xterm/xterm";
import { findBufferLinks } from "./bufferLinks";

// Names that say "image". Stepping through the preview should pass log lines
// such as /usr/bin/env by; the engine still checks the bytes of each file it reads.
const imageNamePattern = /\.(?:png|jpe?g|gif|webp|bmp)$/iu;

// imagePathsInBuffer lists the image paths in a terminal's output, oldest
// first and each once, so the preview can step from one to the next.
export function imagePathsInBuffer(buffer: IBuffer): string[] {
  const paths = new Set<string>();
  // findBufferLinks returns the links that touch one row, so a path that wraps
  // is found from each of its rows; the set keeps it once.
  for (let row = 1; row <= buffer.length; row += 1) {
    for (const { match } of findBufferLinks(buffer, row)) {
      if (match.kind === "path" && imageNamePattern.test(match.target)) paths.add(match.target);
    }
  }
  return [...paths];
}

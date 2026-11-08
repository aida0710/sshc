import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(join(__dirname, "..", "index.css"), "utf8");

describe("reduced motion CSS", () => {
  it("does not turn third-party infinite animations into a rapid loop", () => {
    expect(css).toMatch(/:root\[data-motion="reduce"\] \{[\s\S]*animation-iteration-count:\s*1\s*!important/);
  });

  it("follows the attribute the motion setting sets, not the OS query directly", () => {
    expect(css).not.toMatch(/prefers-reduced-motion/);
    expect(css).toMatch(/@custom-variant motion-reduce \(&:where\(:root\[data-motion="reduce"\] \*\)\)/);
  });

  it("fully disables the application skeleton animation", () => {
    expect(css).toMatch(/\.sshc-skeleton\s*\{\s*animation:\s*none/);
  });
});

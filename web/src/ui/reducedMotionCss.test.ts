import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(join(__dirname, "..", "index.css"), "utf8");

describe("reduced motion CSS", () => {
  it("does not turn third-party infinite animations into a rapid loop", () => {
    expect(css).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*animation-iteration-count:\s*1\s*!important/);
  });

  it("fully disables the application skeleton animation", () => {
    expect(css).toMatch(/\.sshc-skeleton\s*\{\s*animation:\s*none/);
  });
});

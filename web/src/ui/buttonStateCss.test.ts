import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(join(__dirname, "..", "index.css"), "utf8");

// 押せない・押せるの見た目は index.css の決まりで全画面に付くので、決まりそのものを確かめる。
describe("button state CSS", () => {
  it("does not light up a disabled control when it is hovered or pressed", () => {
    expect(css).toMatch(/@custom-variant hover \{[\s\S]*?&:hover:where\(:not\(:disabled\)\)/);
    expect(css).toMatch(/@custom-variant active \(&:active:where\(:not\(:disabled\)\)\)/);
  });

  it("fades every disabled button the same way, except one that is working on a press", () => {
    expect(css).toMatch(/:where\(button\):disabled \{\s*cursor: not-allowed;\s*opacity: 0\.5;/);
    expect(css).toMatch(/:where\(button\[aria-busy="true"\]\):disabled,\s*:where\(fieldset:disabled\) :where\(button\):disabled \{\s*opacity: 1;/);
  });

  it("answers a press on desktop as well as on touch", () => {
    const mobileOnly = css.match(/@media \(max-width: 767px\), \(pointer: coarse\) \{[\s\S]*?\n\}/)?.[0] ?? "";
    expect(mobileOnly).not.toMatch(/:active/);
    // filter は中の absolute の基準を変えて、押した場所からクリックを外すので使わない。
    expect(css).toMatch(/summary\):not\(:disabled\)[^{]*:active \{\s*opacity: 0\.85;/);
  });
});

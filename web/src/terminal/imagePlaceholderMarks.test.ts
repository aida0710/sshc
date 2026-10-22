import { describe, expect, it } from "vitest";

import { createImagePlaceholderMarkFilter } from "./imagePlaceholderMarks";

// One cell as Claude Code writes it under an iTerm2 image: U+2800, then the
// marks for the row, the column and the three digits of the image id.
const placeholderCell = "⠀̅̍̅̅̎";

describe("createImagePlaceholderMarkFilter", () => {
  it("leaves one blank braille cell for each placeholder cell", () => {
    const filter = createImagePlaceholderMarkFilter(() => true);
    expect(filter(`a${placeholderCell}${placeholderCell}b`)).toBe("a⠀⠀b");
  });

  it("drops marks that arrive in the chunk after their U+2800", () => {
    const filter = createImagePlaceholderMarkFilter(() => true);
    expect(filter("a⠀")).toBe("a⠀");
    expect(filter("̅̍")).toBe("");
    expect(filter("̅b̅")).toBe("b̅");
  });

  it("keeps marks on other characters", () => {
    const filter = createImagePlaceholderMarkFilter(() => true);
    expect(filter("é ゙")).toBe("é ゙");
  });

  it("passes everything through when disabled", () => {
    const filter = createImagePlaceholderMarkFilter(() => false);
    expect(filter(placeholderCell)).toBe(placeholderCell);
  });
});

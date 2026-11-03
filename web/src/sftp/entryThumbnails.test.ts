import { describe, expect, it } from "vitest";
import { entriesShowThumbnails, hasThumbnail, maxThumbnailedFilesInFolder } from "./entryThumbnails";

const file = (name: string) => ({ type: "file" as const, name });

describe("hasThumbnail", () => {
  it.each(["a.png", "a.PNG", "a.jpg", "a.jpeg", "a.gif"])("asks the engine to shrink %s", (name) => {
    expect(hasThumbnail(file(name))).toBe(true);
  });

  it.each(["a.svg", "a.webp", "a.heic", "a.mp4", "a.txt", "png"])("leaves %s with its icon", (name) => {
    expect(hasThumbnail(file(name))).toBe(false);
  });

  it("leaves a symlink and a folder with their icons, whatever they are called", () => {
    expect(hasThumbnail({ type: "symlink", name: "a.png" })).toBe(false);
    expect(hasThumbnail({ type: "directory", name: "a.png" })).toBe(false);
  });
});

describe("entriesShowThumbnails", () => {
  it("draws no pictures in a folder that has none", () => {
    expect(entriesShowThumbnails([file("notes.txt")])).toBe(false);
  });

  it("draws pictures up to the limit and none above it", () => {
    const pictures = Array.from({ length: maxThumbnailedFilesInFolder }, (_, index) => file(`${index}.jpg`));
    expect(entriesShowThumbnails(pictures)).toBe(true);
    expect(entriesShowThumbnails([...pictures, file("one-more.jpg")])).toBe(false);
  });
});

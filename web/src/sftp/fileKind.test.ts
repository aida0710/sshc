import { describe, expect, it } from "vitest";
import { fileKindOf } from "./fileKind";

describe("fileKindOf", () => {
  it("tells pictures, recordings, archives and text apart by the name's extension, in any case", () => {
    expect(fileKindOf("IMG_2041.JPG")).toBe("image");
    expect(fileKindOf("demo-recording.mp4")).toBe("video");
    expect(fileKindOf("voice.m4a")).toBe("audio");
    expect(fileKindOf("backup.tar.gz")).toBe("archive");
    expect(fileKindOf("notes.md")).toBe("text");
  });

  it("names no kind for a file without a known extension", () => {
    expect(fileKindOf("Makefile")).toBeNull();
    expect(fileKindOf("data.bin")).toBeNull();
    expect(fileKindOf(".bashrc")).toBeNull();
  });
});

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const webDirectory = join(dirname(fileURLToPath(import.meta.url)), "..");
const scannedDirectories = ["src", "e2e"];
const sourceExtensions = /\.(ts|tsx|css|html|json|mjs)$/;
// Tab, LF and CR are ordinary text. Any other C0 control or DEL written raw
// makes git treat the file as binary, which hides its diff from review and its
// lines from rg; write such characters as escapes (for example "\u0000").
const rawControlCharacter = /[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/;

function sourceFiles(): string[] {
  return scannedDirectories.flatMap((directory) =>
    readdirSync(join(webDirectory, directory), { recursive: true, encoding: "utf8" })
      .filter((path) => sourceExtensions.test(path))
      .map((path) => join(webDirectory, directory, path)),
  );
}

describe("web sources", () => {
  it("contain no raw control characters, so git shows their diffs as text", () => {
    const offenders = sourceFiles()
      .filter((path) => rawControlCharacter.test(readFileSync(path, "utf8")))
      .map((path) => relative(webDirectory, path));

    expect(offenders).toEqual([]);
  });
});

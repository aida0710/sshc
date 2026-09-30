import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { catalogueDifference } from "./catalogue";
import { en, messages } from "./messages";

const sourceDirectory = join(dirname(fileURLToPath(import.meta.url)), "..");
const catalogueDirectory = join("i18n", "messages");

// The screens' own sources: a test may name a key that nothing shows, and the
// catalogues name every key by definition.
function screenSources(): string {
  return readdirSync(sourceDirectory, { recursive: true, encoding: "utf8" })
    .filter((path) => /\.tsx?$/.test(path) && !/\.test\.tsx?$/.test(path) && !path.startsWith(catalogueDirectory))
    .map((path) => readFileSync(join(sourceDirectory, path), "utf8"))
    .join("\n");
}

describe("i18n catalogue coverage", () => {
  it("lists missing and extra keys relative to the English master", () => {
    expect(catalogueDifference(
      { alpha: "Alpha", beta: "Beta" },
      { beta: "ベータ", gamma: "ガンマ" },
    )).toEqual({ missing: ["alpha"], extra: ["gamma"] });
  });

  it("keeps every registered language aligned with the English master", () => {
    const problems = Object.entries(messages)
      .filter(([locale]) => locale !== "en")
      .map(([locale, catalogue]) => ({ locale, ...catalogueDifference(en, catalogue) }))
      .filter(({ missing, extra }) => missing.length > 0 || extra.length > 0);

    expect(problems).toEqual([]);
  });

  // A key built from a template (`sync.direction.${direction}`) cannot be
  // counted, so a family of keys is written out in a Record typed by the value
  // it describes (see sync/syncMessageKeys.ts), and every key then appears in
  // full somewhere in the sources.
  it("has no key that no screen names, so a stale message is removed rather than edited", () => {
    const sources = screenSources();
    const unnamed = Object.keys(en).filter((key) => !sources.includes(`"${key}"`));

    expect(unnamed).toEqual([]);
  });
});

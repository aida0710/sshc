import { describe, expect, it } from "vitest";
import corpus from "./shortcutCorpus.generated.json";
import { isValidShortcutChord } from "./shortcutChord";

describe("ショートカットの和音: engine と同じ答えを出す", () => {
  for (const item of corpus.chord) {
    it(`${item.valid ? "受け入れる" : "断る"}: ${JSON.stringify(item.input)}: ${item.why}`, () => {
      expect(isValidShortcutChord(item.input)).toBe(item.valid);
    });
  }
});

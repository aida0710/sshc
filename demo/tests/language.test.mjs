import assert from "node:assert/strict";
import test from "node:test";
import { chooseLanguage } from "../src/language.js";
import { englishMessages } from "../src/messages-en.js";
import { japaneseMessages } from "../src/messages-ja.js";

test("保存した言語を、ブラウザの言語より優先する", () => {
  assert.equal(chooseLanguage({ saved: "en", browserLanguages: ["ja-JP"] }), "en");
  assert.equal(chooseLanguage({ saved: "ja", browserLanguages: ["en-US"] }), "ja");
});

test("保存がなければブラウザの言語を順に見て、どれも対応外なら英語にする", () => {
  assert.equal(chooseLanguage({ saved: null, browserLanguages: ["fr-FR", "ja-JP", "en-US"] }), "ja");
  assert.equal(chooseLanguage({ saved: null, browserLanguages: ["EN-gb"] }), "en");
  assert.equal(chooseLanguage({ saved: "de", browserLanguages: ["fr-FR"] }), "en");
  assert.equal(chooseLanguage({ saved: null, browserLanguages: [] }), "en");
});

test("日本語と英語のカタログは同じ項目を同じ形で持つ", () => {
  const shape = (catalog) => Object.fromEntries(Object.entries(catalog).map(([key, value]) => [key,
    typeof value === "function" ? `function/${value.length}`
      : Array.isArray(value) ? `array/${value.length}`
      : typeof value === "object" ? `object/${Object.keys(value).sort().join(",")}` : typeof value]));
  assert.deepEqual(shape(englishMessages), shape(japaneseMessages));
});

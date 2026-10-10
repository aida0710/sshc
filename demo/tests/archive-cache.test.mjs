import assert from "node:assert/strict";
import test from "node:test";
import { archiveEntryURL } from "../src/archive-cache.js";

const baseURL = new URL("https://demo.test/github-releases/v1.2.3/");

test("展開するファイルはデモの版のフォルダの中に置く", () => {
  assert.equal(archiveEntryURL("ui/index.html", baseURL).href, "https://demo.test/github-releases/v1.2.3/ui/index.html");
});

test("スキーム付きの名前でフォルダの外や別オリジンへ出るファイルを拒否する", () => {
  for (const path of ["http:evil.test/x.js", "javascript:alert(1)", "data:text/html,x", "//evil.test/x"]) {
    assert.throws(() => archiveEntryURL(path, baseURL), /escapes its folder/, path);
  }
  // The same scheme as the page is read as a relative name and therefore stays inside the folder.
  assert.equal(archiveEntryURL("https:evil.test/index.html", baseURL).href,
    "https://demo.test/github-releases/v1.2.3/evil.test/index.html");
});

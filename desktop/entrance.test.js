"use strict";

const { test } = require("node:test");
const assert = require("node:assert");
const { parseEntrance } = require("./entrance");

// **エンジンは入口を 1 行で書き出す。** 起こした本人だけがそのパイプを持つ。
test("takes the entrance out of what the engine printed", () => {
  const text = "sshc: listening\nhttp://127.0.0.1:52683/?token=abc\n";
  assert.strictEqual(parseEntrance(text), "http://127.0.0.1:52683/?token=abc");
});

// **ループバック以外は入口ではない。** ここを緩めると、エンジンが答えた
// つもりの何かを窓に読ませることになる。
test("refuses an address that is not loopback", () => {
  assert.strictEqual(parseEntrance("http://example.com/\n"), null);
});

test("answers null until the line has arrived", () => {
  assert.strictEqual(parseEntrance("sshc: starting\n"), null);
});

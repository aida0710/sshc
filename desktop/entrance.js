"use strict";

/**
 * parseEntrance は、エンジンが書き出した入口の URL を取り出す。
 *
 * **起こした本人が、起こした子のパイプから受け取る。** ファイルを経由しない
 * のは、あの 1 行に有効な bootstrap トークンが乗っているからである。
 */
function parseEntrance(text) {
  for (const line of String(text).split("\n")) {
    const candidate = line.trim();
    if (candidate.startsWith("http://127.0.0.1:")) return candidate;
  }
  return null;
}

module.exports = { parseEntrance };

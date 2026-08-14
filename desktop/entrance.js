"use strict";

/**
 * parseEntrance は、エンジンが書き出した入口の URL を取り出す。
 *
 * **起こした本人が、起こした子のパイプから受け取る。** ファイルを経由しない
 * のは、あの 1 行に有効な bootstrap トークンが乗っているからである。
 */
function parseEntrance(text) {
  const raw = String(text);
  const lines = raw.split("\n");
  // **改行がまだ来ていない最後の断片は、行として見ない。** あの 1 行は
  // bootstrap トークンを乗せる機微な行であり、届いている途中のものを
  // 読む余地を残さない。
  if (!raw.endsWith("\n")) lines.pop();
  for (const line of lines) {
    const candidate = line.trim();
    if (candidate.startsWith("http://127.0.0.1:")) return candidate;
  }
  return null;
}

module.exports = { parseEntrance };

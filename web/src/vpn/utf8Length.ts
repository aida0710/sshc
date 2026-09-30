// utf8Length は、文字列の UTF-8 のバイト数である。engine（Go）は長さの上限を len、つまり
// UTF-8 のバイト数で数えるので、画面の上限の検査もこれで数える。
export function utf8Length(text: string): number {
  return new TextEncoder().encode(text).length;
}

// ここにあるのはテストのための道具である。**本番の経路からは呼ばれない。**

/**
 * sentJson は、fetch の偽物が受け取った本文を JSON として読む。
 *
 * **String(init.body) と書いてはならない。** body の型は BodyInit であり、
 * Blob や FormData や URLSearchParams でもありうる——それらを String() に渡すと
 * `[object Object]` という文字列になり、**JSON.parse がそこで落ちる。** 落ちた
 * テストが言うのは「Unexpected token o」だけで、本当の理由（送っているものが
 * 文字列ではない）はどこにも出ない。
 *
 * このアプリが送る本文は必ず JSON の文字列なので、そうでないならテストが
 * 確かめたい要求をそもそも送っていない。ここで名指しで断る。
 */
export function sentJson(init: RequestInit): unknown {
  const { body } = init;
  if (typeof body !== "string") {
    throw new Error(`the request body is ${typeof body}, not a JSON string`);
  }
  return JSON.parse(body);
}

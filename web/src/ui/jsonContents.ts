// sortKeys は、オブジェクトのキーを名前順に並べ直した値を返す。項目を消して足し直すとキーの順が変わるので、
// 並べ直さないと同じ内容でも JSON が一致しない。
export function sortKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (value === null || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .filter(([, item]) => item !== undefined)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => [key, sortKeys(item)]),
  );
}

// sameContents は、2つの値を JSON に書いたときの内容が、キーの順を除いて同じかを返す。
export function sameContents(first: unknown, second: unknown): boolean {
  return JSON.stringify(sortKeys(first)) === JSON.stringify(sortKeys(second));
}

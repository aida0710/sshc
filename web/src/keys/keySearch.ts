import type { KeyItem } from "./api";
import { includeKeyPairContext } from "./organizer";

// matchesKeyQuery は、パス・種類・アルゴリズム・フィンガープリント・参照している
// ホストのパターンのどれかが検索語を含むかを返す。query は小文字にしてから渡す。
function matchesKeyQuery(item: KeyItem, query: string): boolean {
  return (
    item.relativePath.toLowerCase().includes(query) ||
    item.kind.toLowerCase().includes(query) ||
    item.algorithm.toLowerCase().includes(query) ||
    item.fingerprint.toLowerCase().includes(query) ||
    item.references.some((reference) =>
      reference.hostPatterns.some((pattern) => pattern.toLowerCase().includes(query)),
    )
  );
}

// searchKeyItems は、検索語に当たった鍵を、対になる秘密鍵・公開鍵・証明書と一緒に返す。
// 検索語が空なら items をそのまま返す。
export function searchKeyItems(items: KeyItem[], rawQuery: string): KeyItem[] {
  const query = rawQuery.trim().toLowerCase();
  if (query === "") return items;
  return includeKeyPairContext(items, items.filter((item) => matchesKeyQuery(item, query)));
}

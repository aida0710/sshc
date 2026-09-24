import type { HostMetadata } from "../api/config";

// sshcタブの下書き（接続ごとの HostMetadata）を比べ、組み立てるための関数。

// isOmitted は、engine が保存しない値かを返す。engine の HostMetadata は各項目が
// omitempty なので、空文字、0、false、空の配列は、項目が無いのと同じ内容で保存される。
function isOmitted(value: unknown): boolean {
  return value === undefined || value === null || value === "" || value === 0 || value === false ||
    (Array.isArray(value) && value.length === 0);
}

// sortKeys は、オブジェクトのキーを名前順に並べ直した値を返す。項目を消して足し直すと
// キーの順が変わるので、並べ直さないと同じ内容でも JSON が一致しない。
function sortKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (value === null || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .filter(([, item]) => item !== undefined)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => [key, sortKeys(item)]),
  );
}

// canonicalHostMetadata は、保存したときの内容を表す文字列である。保存すると同じ内容に
// なる2つの設定は、同じ文字列になる。
export function canonicalHostMetadata(metadata: HostMetadata): string {
  const kept = Object.entries(metadata).filter(([, value]) => !isOmitted(value));
  return JSON.stringify(sortKeys(Object.fromEntries(kept)));
}

export function sameHostMetadata(left: HostMetadata, right: HostMetadata): boolean {
  return canonicalHostMetadata(left) === canonicalHostMetadata(right);
}

type OptionalChoice = "encoding" | "osc52" | "vpn";

// withOptionalChoice は、選択肢の項目を変えた下書きを返す。空の選択（UTF-8、全体の設定に
// 従う、VPNを使わない）は、項目そのものを消す。
export function withOptionalChoice<Key extends OptionalChoice>(
  metadata: HostMetadata,
  key: Key,
  value: NonNullable<HostMetadata[Key]> | "",
): HostMetadata {
  const next: HostMetadata = { ...metadata };
  if (value === "") delete next[key];
  else next[key] = value;
  return next;
}

// parseTags は、カンマ区切りで入力したタグから、前後の空白と空のタグを除いて返す。
export function parseTags(text: string): string[] {
  return text
    .split(",")
    .map((tag) => tag.trim())
    .filter((tag) => tag !== "");
}

export function formatTags(tags: readonly string[] | undefined): string {
  return (tags ?? []).join(", ");
}

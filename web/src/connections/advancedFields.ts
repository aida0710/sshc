import type { FieldEdit, FormField } from "../api/config";
import { forwardKindForKeyword } from "../forwarding/portForwarding";

// 踏み台・転送・その他のタブは、Host ブロックのディレクティブを3つの節に分けて出す。
export type AdvancedPart = "jump" | "forwards" | "other";

// 基本のタブが自分の欄で編集するディレクティブ。ここには出さない。ただし2回書かれているものは、
// 基本のタブが2つ目を出せないので、消せるようにその他の設定に残す。
const basicFormKeywords = new Set(["hostname", "user", "port", "identityfile"]);

export function isForwardKeyword(keyword: string): boolean {
  return forwardKindForKeyword(keyword) !== undefined;
}

export function partOf(field: FormField): AdvancedPart | null {
  if (isForwardKeyword(field.keyword)) return "forwards";
  if (field.category === "jump") return "jump";
  if (basicFormKeywords.has(field.keyword.toLowerCase()) && field.duplicate !== true) return null;
  return "other";
}

export function fieldsIn(fields: readonly FormField[], part: AdvancedPart): FormField[] {
  return fields.filter((field) => partOf(field) === part);
}

export function isForwardAddition(edit: FieldEdit): boolean {
  return edit.action === "add" && isForwardKeyword(edit.keyword ?? "");
}

export function hasProxyJump(fields: readonly FormField[], additions: readonly FieldEdit[]): boolean {
  const named = (keyword: string | undefined) => keyword?.toLowerCase() === "proxyjump";
  return fields.some((field) => named(field.keyword)) || additions.some((edit) => named(edit.keyword));
}

// forwardEnds は、転送の値を、待ち受けと転送先に分ける（SOCKS プロキシには転送先が無い）。
export function forwardEnds(values: readonly string[]): { listen: string; target: string } {
  return { listen: values[0] ?? "", target: values.slice(1).join(" ") };
}

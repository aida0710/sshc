import type { GroupMetadata } from "../api/config";
import type { MessageKey } from "../i18n/messages";
import { formatDirectiveValues, parseDirectiveValues } from "../rules/rules";

// グループの編集欄の下書きと、グループの共通設定（metadata の settings）の行き来を決める。

export type GroupSetting = NonNullable<GroupMetadata["settings"]>[number];

export type DefaultField = { keyword: string; label: MessageKey; placeholder: string };

// 既定値として欄を用意する項目。接続先の編集でよく使うものだけを並べ、残りは「その他の設定」に置く。
export const defaultFields: readonly DefaultField[] = [
  { keyword: "User", label: "groupEditor.user", placeholder: "deploy" },
  { keyword: "Port", label: "groupEditor.port", placeholder: "22" },
  { keyword: "IdentityFile", label: "groupEditor.identityFile", placeholder: "~/.ssh/keys/id_ed25519" },
  { keyword: "ProxyJump", label: "groupEditor.proxyJump", placeholder: "bastion" },
];

export type GroupDraft = {
  colour: string;
  // 欄で編集する値。グループの設定に無いか1回だけある項目だけを欄にする。
  defaults: Record<string, string>;
  // 欄にしない設定。元からある設定は同じオブジェクトのまま持ち、保存のときに元の位置へ戻す。
  other: GroupSetting[];
};

function sameKeyword(left: string, right: string): boolean {
  return left.toLowerCase() === right.toLowerCase();
}

// repeatedFieldKeywords は、グループの設定に2回以上ある欄の項目を返す（IdentityFile を2つ指定するなど）。
// 欄は1つの値しか持てないので、これらは「その他の設定」で編集する。
export function repeatedFieldKeywords(group: GroupMetadata): ReadonlySet<string> {
  const settings = group.settings ?? [];
  return new Set(defaultFields
    .filter((field) => settings.filter((setting) => sameKeyword(setting.keyword, field.keyword)).length > 1)
    .map((field) => field.keyword));
}

function fieldFor(keyword: string, repeated: ReadonlySet<string>): DefaultField | undefined {
  const field = defaultFields.find((candidate) => sameKeyword(candidate.keyword, keyword));
  return field === undefined || repeated.has(field.keyword) ? undefined : field;
}

export function draftOf(group: GroupMetadata): GroupDraft {
  const repeated = repeatedFieldKeywords(group);
  const defaults: Record<string, string> = {};
  const other: GroupSetting[] = [];
  for (const setting of group.settings ?? []) {
    const field = fieldFor(setting.keyword, repeated);
    if (field === undefined) other.push(setting);
    else defaults[field.keyword] = formatDirectiveValues(setting.keyword, setting.values) ?? "";
  }
  return { colour: group.colour ?? "", defaults, other };
}

// fieldSetting は、欄の文字列から設定を作る。空なら設定を消し、元の値と同じ文字列なら元の設定をそのまま残す。
function fieldSetting(field: DefaultField, text: string, previous?: GroupSetting): GroupSetting[] {
  const trimmed = text.trim();
  if (trimmed === "") return [];
  if (previous !== undefined && formatDirectiveValues(previous.keyword, previous.values) === trimmed) return [previous];
  return [{ keyword: previous?.keyword ?? field.keyword, values: parseDirectiveValues(field.keyword, trimmed) }];
}

// groupFrom は、下書きからグループの設定を作る。元からある設定は元の位置に置き、新しく入れた欄の値と
// 「その他の設定」に足したものは最後に足す。値の引用符が閉じていなければ parseDirectiveValues が例外を投げる。
export function groupFrom(group: GroupMetadata, draft: GroupDraft): GroupMetadata {
  const original = group.settings ?? [];
  const repeated = repeatedFieldKeywords(group);
  const kept = new Set(draft.other);
  const placed = new Set<string>();
  const settings: GroupSetting[] = [];
  for (const setting of original) {
    const field = fieldFor(setting.keyword, repeated);
    if (field !== undefined) {
      placed.add(field.keyword);
      settings.push(...fieldSetting(field, draft.defaults[field.keyword] ?? "", setting));
    } else if (kept.has(setting)) {
      settings.push(setting);
    }
  }
  for (const field of defaultFields) {
    if (placed.has(field.keyword) || repeated.has(field.keyword)) continue;
    settings.push(...fieldSetting(field, draft.defaults[field.keyword] ?? ""));
  }
  settings.push(...draft.other.filter((setting) => !original.includes(setting)));

  const next: GroupMetadata = { ...group, settings };
  if (draft.colour === "") delete next.colour;
  else next.colour = draft.colour;
  if (settings.length === 0) delete next.settings;
  return next;
}

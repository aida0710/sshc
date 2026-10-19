import { useEffect, useState } from "react";
import type { GroupMetadata } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { formatDirectiveValues, parseDirectiveValues } from "../rules/rules";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button, Card, Notice } from "../ui/surface";
import { DraftSaveBar } from "./DraftSaveBar";

type Setting = NonNullable<GroupMetadata["settings"]>[number];

// 既定値として欄を用意する項目。接続先の編集でよく使うものだけを並べ、残りは「その他の設定」に置く。
const defaultFields: readonly { keyword: string; label: MessageKey; placeholder: string }[] = [
  { keyword: "User", label: "groupEditor.user", placeholder: "deploy" },
  { keyword: "Port", label: "groupEditor.port", placeholder: "22" },
  { keyword: "IdentityFile", label: "groupEditor.identityFile", placeholder: "~/.ssh/keys/id_ed25519" },
  { keyword: "ProxyJump", label: "groupEditor.proxyJump", placeholder: "bastion" },
];

const fieldKeywords = new Set(defaultFields.map((field) => field.keyword.toLowerCase()));

type GroupDraft = {
  colour: string;
  defaults: Record<string, string>;
  other: Setting[];
};

function draftOf(group: GroupMetadata): GroupDraft {
  const defaults: Record<string, string> = {};
  const other: Setting[] = [];
  for (const setting of group.settings ?? []) {
    const field = defaultFields.find((candidate) => candidate.keyword.toLowerCase() === setting.keyword.toLowerCase());
    if (field === undefined) other.push(setting);
    else defaults[field.keyword] = formatDirectiveValues(setting.keyword, setting.values) ?? "";
  }
  return { colour: group.colour ?? "", defaults, other };
}

function groupFrom(group: GroupMetadata, draft: GroupDraft): GroupMetadata {
  const settings: Setting[] = [];
  for (const field of defaultFields) {
    const text = (draft.defaults[field.keyword] ?? "").trim();
    if (text !== "") settings.push({ keyword: field.keyword, values: parseDirectiveValues(field.keyword, text) });
  }
  settings.push(...draft.other);
  const next: GroupMetadata = { ...group, settings };
  if (draft.colour === "") delete next.colour;
  else next.colour = draft.colour;
  if (settings.length === 0) delete next.settings;
  return next;
}

type GroupEditorProps = {
  group: GroupMetadata;
  memberCount: number;
  actions: ActionMenuItem[];
  onSave: (next: GroupMetadata) => Promise<boolean>;
  // 保存していない変更があるかを知らせる。画面はほかへ移る前に捨ててよいかを確かめる。
  onDirtyChange: (dirty: boolean) => void;
};

// GroupEditor は、グループを選んで接続先を開いていないときに右側に出す、グループの編集欄である。
// SSH の設定はキーワードを打ち込ませず、接続先の編集と同じ名前の欄で既定値として見せる。
export function GroupEditor({ group, memberCount, actions, onSave, onDirtyChange }: GroupEditorProps) {
  const t = useTranslate();
  const [saved, setSaved] = useState(() => draftOf(group));
  const [draft, setDraft] = useState(saved);
  const [newKeyword, setNewKeyword] = useState("");
  const [newValue, setNewValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [draftError, setDraftError] = useState("");
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved);
  const parent = group.name.includes("/") ? group.name.slice(0, group.name.lastIndexOf("/") + 1) : "";
  const label = group.name.slice(group.name.lastIndexOf("/") + 1);

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);
  // 編集欄が閉じたら（別のグループや接続先へ移ったら）、下書きはもう無い。
  useEffect(() => () => onDirtyChange(false), [onDirtyChange]);

  function addOther() {
    const keyword = newKeyword.trim();
    if (keyword === "" || fieldKeywords.has(keyword.toLowerCase())) return;
    let values: string[];
    try {
      values = parseDirectiveValues(keyword, newValue);
    } catch {
      setDraftError(t("groups.unbalancedQuote"));
      return;
    }
    setDraftError("");
    setDraft({ ...draft, other: [...draft.other, { keyword, values }] });
    setNewKeyword("");
    setNewValue("");
  }

  async function save() {
    let next: GroupMetadata;
    try {
      next = groupFrom(group, draft);
    } catch {
      setDraftError(t("groups.unbalancedQuote"));
      return;
    }
    setDraftError("");
    setSaving(true);
    const ok = await onSave(next);
    setSaving(false);
    if (ok) setSaved(draft);
  }

  return (
    <Card as="section" radius="md" aria-labelledby="group-editor-heading" className="flex flex-col gap-5 p-5">
      <header className="flex items-start gap-3">
        <span
          aria-hidden="true"
          className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md bg-surface-subtle"
          style={draft.colour === "" ? undefined : { color: draft.colour }}
        >
          <Icon name="groups" className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-xs text-ink-faint">{t("groupEditor.label")}</p>
          <h2 id="group-editor-heading" className="truncate text-base font-semibold tracking-tight text-ink">
            {parent === "" ? null : <span className="font-normal text-ink-faint">{parent}</span>}
            {label}
          </h2>
          <p className="mt-0.5 text-xs text-ink-muted">{t("groupEditor.members", { count: memberCount })}</p>
        </div>
        <ActionMenu
          label={t("tree.groupActions", { name: group.name })}
          items={actions}
          triggerClassName="flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
        />
      </header>

      <div className="flex flex-col gap-3 border-t border-line pt-5">
        <h3 className={sectionHeading}>{t("groupEditor.defaults")}</h3>
        <p className={hintText}>{t("groupEditor.defaultsHint")}</p>
        <div className="grid gap-3 sm:grid-cols-2">
          {defaultFields.map((field) => (
            <Field key={field.keyword} label={t(field.label)}>
              <input
                value={draft.defaults[field.keyword] ?? ""}
                placeholder={`${t("groupEditor.notSet")}（${field.placeholder}）`}
                onChange={(event) => setDraft({ ...draft, defaults: { ...draft.defaults, [field.keyword]: event.currentTarget.value } })}
                className={control}
              />
            </Field>
          ))}
        </div>
      </div>

      <details className="border-t border-line pt-4" open={draft.other.length > 0}>
        <summary className={`cursor-pointer ${sectionHeading}`}>{t("groupEditor.other")}</summary>
        <ul className="mt-3 flex flex-col gap-1.5">
          {draft.other.map((setting, index) => (
            <li key={`${setting.keyword}-${index}`} className="flex items-center gap-2 rounded-md bg-surface-subtle px-2 py-1.5 font-mono text-xs text-ink">
              <span className="min-w-0 flex-1 truncate">{`${setting.keyword} ${formatDirectiveValues(setting.keyword, setting.values) ?? ""}`}</span>
              <button
                type="button"
                aria-label={t("groupEditor.removeOther", { keyword: setting.keyword })}
                onClick={() => setDraft({ ...draft, other: draft.other.filter((_, position) => position !== index) })}
                className="flex size-7 items-center justify-center rounded text-ink-faint hover:text-danger"
              >
                <Icon name="close" className="size-3.5" />
              </button>
            </li>
          ))}
        </ul>
        <div className="mt-3 flex flex-wrap items-end gap-2">
          <div className="min-w-40 flex-1">
            <Field label={t("groupEditor.keyword")}>
              <input value={newKeyword} placeholder="ServerAliveInterval" onChange={(event) => setNewKeyword(event.currentTarget.value)} className={control} />
            </Field>
          </div>
          <div className="min-w-32 flex-1">
            <Field label={t("groupEditor.value")}>
              <input value={newValue} placeholder="30" onChange={(event) => setNewValue(event.currentTarget.value)} className={control} />
            </Field>
          </div>
          <Button disabled={newKeyword.trim() === ""} onClick={addOther}>{t("groupEditor.addOther")}</Button>
        </div>
      </details>

      <div className="flex items-center gap-3 border-t border-line pt-4">
        <label className="flex items-center gap-2 text-sm text-ink-muted">
          {t("groupEditor.colour")}
          <input
            type="color"
            value={draft.colour === "" ? "#8e8e93" /* palette-exempt: ネイティブコントロール自身の中立色 */ : draft.colour}
            onChange={(event) => setDraft({ ...draft, colour: event.currentTarget.value })}
            className="h-8 w-12 rounded-md border border-control-line bg-control"
          />
        </label>
      </div>

      {draftError === "" ? null : <Notice tone="danger">{draftError}</Notice>}
      {dirty ? (
        <DraftSaveBar
          saveLabel={t("groupEditor.save")}
          saveDisabled={saving}
          discardDisabled={saving}
          saving={saving}
          onDiscard={() => setDraft(saved)}
          onSave={() => void save()}
        />
      ) : null}
    </Card>
  );
}

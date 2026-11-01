import { useCallback, useEffect, useMemo, useState } from "react";
import type { FieldEdit, FormField, HostDetail } from "../api/config";
import { useTranslate } from "../i18n/context";
import { forwardDirective, forwardHintKeys, forwardLabelKeys, forwardKindForKeyword, validForwardDestination, validForwardPort, type PortForwardKind } from "../forwarding/portForwarding";
import type { AdvancedArea } from "../routing/connectionRoute";
import { control, hintText } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { Icon } from "../ui/icons";
import { Button, Notice } from "../ui/surface";
import { formatDirectiveValues, parseDirectiveValues } from "../rules/rules";
import { fieldsIn, forwardEnds, hasProxyJump, isForwardAddition } from "./advancedFields";
import { draftResetKey } from "./draftResetKey";
import { DraftSaveBar } from "./DraftSaveBar";
import { EditorSection } from "./EditorSection";
import { HostBlockText } from "./HostBlockText";
import { useDraftSave } from "./useDraftSave";
import { useReportDirty } from "./useReportDirty";

type AdvancedSettingsProps = {
  detail: HostDetail;
  area: AdvancedArea;
  onAreaChange: (area: AdvancedArea) => void;
  // onFieldEdits と onBlockRaw は、下書きを保存する。保存できなかったときは reject する。
  onFieldEdits: (edits: FieldEdit[]) => Promise<void>;
  onBlockRaw: (raw: string) => Promise<void>;
  disabled: boolean;
  onDirtyChange: (dirty: boolean) => void;
  onDiscardReady?: ((discard: (() => void) | null) => void) | undefined;
};

function fieldKey(field: FormField): string {
  return `${field.line}-${field.keyword}`;
}

const proxyJumpKeyword = "ProxyJump";

// 1行の追加欄と表の行の、小さな文字の入力欄。
const compactControl = `${control} h-[30px] py-0 text-[12.5px]`;

// AdvancedSettings は、接続エディタの「踏み台・転送・その他」のタブである。踏み台・ポート転送・その他の設定の
// 3つの節に、Host ブロックのディレクティブを並べる。area が Raw のときは、ブロックの文を直接編集する。
// ディレクティブの下書きと文の下書きは、同時には持たない（片方を変えている間はもう片方を止める）。
export function AdvancedSettings({
  detail,
  area,
  onAreaChange,
  onFieldEdits,
  onBlockRaw,
  disabled,
  onDirtyChange,
  onDiscardReady,
}: AdvancedSettingsProps) {
  const t = useTranslate();
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [removed, setRemoved] = useState<number[]>([]);
  const [additions, setAdditions] = useState<FieldEdit[]>([]);
  const [newJump, setNewJump] = useState("");
  const [newKeyword, setNewKeyword] = useState("");
  const [newValue, setNewValue] = useState("");
  const [newForwardKind, setNewForwardKind] = useState<PortForwardKind>("local");
  const [newListenPort, setNewListenPort] = useState("");
  const [newDestination, setNewDestination] = useState("");
  const [blockRaw, setBlockRaw] = useState(detail.form.raw);
  const [localError, setLocalError] = useState("");

  const discard = useCallback(() => {
    setDrafts({});
    setRemoved([]);
    setAdditions([]);
    setNewJump("");
    setNewKeyword("");
    setNewValue("");
    setNewForwardKind("local");
    setNewListenPort("");
    setNewDestination("");
    setBlockRaw(detail.form.raw);
    setLocalError("");
  }, [detail.form.raw]);

  const resetKey = draftResetKey(detail.form.entry.identity, detail.file.contents);
  useEffect(() => {
    discard();
    // resetKey already names the identity and file contents this draft is
    // based on; watching `detail` itself would restart the form on every
    // refetch of the same revision.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  const jumpFields = fieldsIn(detail.form.fields, "jump");
  const forwardFields = fieldsIn(detail.form.fields, "forwards");
  const otherFields = fieldsIn(detail.form.fields, "other");
  const offerProxyJump = !hasProxyJump(detail.form.fields, additions);
  // draft は、保存する下書き（ディレクティブの変更と文）をまとめたものである。どれかを変えたときだけ
  // 別の参照になるので、useDraftSave が保存した下書きかを見分けられる。
  const draft = useMemo(() => ({ drafts, removed, additions, newJump, blockRaw }), [drafts, removed, additions, newJump, blockRaw]);
  const { saving, written, save } = useDraftSave({ draft, saved: detail });
  const fieldDirty = !written && (removed.length > 0 || additions.length > 0 || newJump.trim() !== "" || Object.entries(drafts).some(([key, value]) => {
    const field = detail.form.fields.find((candidate) => fieldKey(candidate) === key);
    return field !== undefined && value !== formatDirectiveValues(field.keyword, field.values);
  }));
  const rawDirty = !written && blockRaw !== detail.form.raw;
  const dirty = fieldDirty || rawDirty;
  const fieldsDisabled = disabled || saving || rawDirty;

  useReportDirty(dirty, onDirtyChange);

  useEffect(() => {
    onDiscardReady?.(discard);
    return () => onDiscardReady?.(null);
  }, [discard, onDiscardReady]);

  function draftFor(field: FormField): string {
    return drafts[fieldKey(field)] ?? formatDirectiveValues(field.keyword, field.values) ?? "";
  }

  function toggleRemoved(field: FormField) {
    setRemoved(removed.includes(field.line) ? removed.filter((line) => line !== field.line) : [...removed, field.line]);
  }

  function submitFieldEdits() {
    const edits: FieldEdit[] = [];
    try {
      for (const field of detail.form.fields) {
        if (removed.includes(field.line)) {
          edits.push({ action: "remove", line: field.line });
          continue;
        }
        const fieldDraft = drafts[fieldKey(field)];
        if (fieldDraft === undefined || fieldDraft === formatDirectiveValues(field.keyword, field.values)) continue;
        edits.push({ action: "set", line: field.line, values: parseDirectiveValues(field.keyword, fieldDraft) });
      }
      if (newJump.trim() !== "") edits.push({ action: "add", keyword: proxyJumpKeyword, values: parseDirectiveValues(proxyJumpKeyword, newJump.trim()) });
      edits.push(...additions);
    } catch {
      setLocalError(t("host.unbalancedQuote"));
      return;
    }
    if (edits.length === 0) return;
    setLocalError("");
    void save(() => onFieldEdits(edits));
  }

  function addDirective() {
    if (newKeyword.trim() === "") {
      setLocalError(t("host.needsKeyword"));
      return;
    }
    try {
      setAdditions([...additions, { action: "add", keyword: newKeyword.trim(), values: parseDirectiveValues(newKeyword.trim(), newValue) }]);
    } catch {
      setLocalError(t("host.unbalancedQuote"));
      return;
    }
    setNewKeyword("");
    setNewValue("");
    setLocalError("");
  }

  function addForward() {
    if (!validForwardPort(newListenPort)) {
      setLocalError(t("conn.forwardInvalidPort"));
      return;
    }
    if (newForwardKind !== "dynamic" && !validForwardDestination(newDestination)) {
      setLocalError(t("conn.forwardInvalidDestination"));
      return;
    }
    setAdditions([...additions, { action: "add", ...forwardDirective(newForwardKind, newListenPort, newDestination) }]);
    setNewListenPort("");
    setNewDestination("");
    setLocalError("");
  }

  if (area === "Raw") {
    return (
      <HostBlockText
        raw={blockRaw}
        dirty={rawDirty}
        onChange={setBlockRaw}
        onBack={() => onAreaChange("Settings")}
        blockedByFields={fieldDirty}
        disabled={disabled || saving || fieldDirty}
        saving={saving}
        onDiscard={discard}
        onSave={() => void save(() => onBlockRaw(blockRaw))}
      />
    );
  }

  const removeButton = (field: FormField) => (
    <RemoveToggle removed={removed.includes(field.line)} disabled={fieldsDisabled} label={field.keyword} onToggle={() => toggleRemoved(field)} />
  );
  const pendingForwards = additions.filter(isForwardAddition);
  const pendingOthers = additions.filter((edit) => !isForwardAddition(edit));

  return (
    <section aria-label={t("conn.advancedLabel")} className="flex flex-col">
      {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}
      {rawDirty ? <Notice>{t("conn.advancedRawBlocksFields")}</Notice> : null}

      <EditorSection title={t("advanced.jumpHeading")}>
        {jumpFields.map((field) => (
          <FieldRow key={fieldKey(field)} density="editor" interactiveChildren label={field.keyword.toLowerCase() === "proxyjump" ? t("advanced.jumpVia") : field.keyword}>
            <span className="flex items-center gap-2">
              <input
                aria-label={field.keyword}
                value={draftFor(field)}
                disabled={fieldsDisabled || !field.editable || removed.includes(field.line)}
                onChange={(event) => setDrafts({ ...drafts, [fieldKey(field)]: event.target.value })}
                className={`${control} font-mono ${removed.includes(field.line) ? "line-through" : ""}`}
              />
              {removeButton(field)}
            </span>
          </FieldRow>
        ))}
        {offerProxyJump ? (
          <FieldRow density="editor" label={t("advanced.jumpVia")}>
            <input value={newJump} disabled={fieldsDisabled} placeholder={t("advanced.jumpNone")} onChange={(event) => setNewJump(event.target.value)} className={`${control} font-mono`} />
          </FieldRow>
        ) : null}
      </EditorSection>

      <EditorSection title={t("conn.portForwarding")}>
        {forwardFields.length === 0 && pendingForwards.length === 0 ? <p className="pb-1 text-[12.5px] text-ink-faint">{t("conn.forwardNoneSaved")}</p> : (
          <table className="w-full table-fixed text-[12.5px]">
            <thead>
              <tr className="text-left text-[11.5px] text-ink-faint">
                <th scope="col" className="w-[7.5rem] pb-1.5 pr-2 font-normal">{t("conn.forwardType")}</th>
                <th scope="col" className="w-[6.5rem] pb-1.5 pr-2 font-normal">{t("advanced.forwardListen")}</th>
                <th scope="col" className="pb-1.5 pr-2 font-normal">{t("conn.forwardDestination")}</th>
                <th scope="col" className="w-14 pb-1.5"><span className="sr-only">{t("advanced.rowActions")}</span></th>
              </tr>
            </thead>
            <tbody>
              {forwardFields.map((field) => {
                const ends = forwardEnds(field.values);
                const gone = removed.includes(field.line);
                return (
                  <tr key={fieldKey(field)} className={`border-t border-line ${gone ? "text-ink-faint line-through" : "text-ink"}`}>
                    <td className="truncate py-1.5 pr-2">{t(forwardLabelKeys[forwardKindForKeyword(field.keyword) ?? "local"])}</td>
                    <td className="truncate py-1.5 pr-2 font-mono">{ends.listen}</td>
                    <td className="truncate py-1.5 pr-2 font-mono">{ends.target}</td>
                    <td className="py-1.5 text-right">{removeButton(field)}</td>
                  </tr>
                );
              })}
              {pendingForwards.map((addition, index) => {
                const ends = forwardEnds(addition.values ?? []);
                return (
                  <tr key={`new-forward-${index}`} className="border-t border-line text-ink">
                    <td className="truncate py-1.5 pr-2">{t(forwardLabelKeys[forwardKindForKeyword(addition.keyword ?? "") ?? "local"])}</td>
                    <td className="truncate py-1.5 pr-2 font-mono">{ends.listen}</td>
                    <td className="truncate py-1.5 pr-2 font-mono">{ends.target}<span className="ml-2 font-sans text-[11px] text-notice-ink">{t("advanced.pending")}</span></td>
                    <td className="py-1.5 text-right"><RemoveToggle removed={false} disabled={fieldsDisabled} label={addition.keyword ?? ""} onToggle={() => setAdditions(additions.filter((candidate) => candidate !== addition))} /></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
        <div className="mt-2 grid gap-2 @sm/editor:grid-cols-[7.5rem_6.5rem_minmax(0,1fr)_auto]">
          <select aria-label={t("conn.forwardType")} value={newForwardKind} disabled={fieldsDisabled} onChange={(event) => setNewForwardKind(event.currentTarget.value as PortForwardKind)} className={compactControl}>
            <option value="local">{t("conn.forwardLocal")}</option>
            <option value="remote">{t("conn.forwardRemote")}</option>
            <option value="dynamic">{t("conn.forwardDynamic")}</option>
          </select>
          <input aria-label={t(newForwardKind === "remote" ? "conn.forwardRemoteListenPort" : "conn.forwardListenPort")} inputMode="numeric" placeholder={t("advanced.portPlaceholder")} value={newListenPort} disabled={fieldsDisabled} onChange={(event) => setNewListenPort(event.currentTarget.value)} className={`${compactControl} font-mono`} />
          {newForwardKind === "dynamic" ? <span /> : (
            <input aria-label={t("conn.forwardDestination")} placeholder="127.0.0.1:5432" value={newDestination} disabled={fieldsDisabled} onChange={(event) => setNewDestination(event.currentTarget.value)} className={`${compactControl} font-mono`} />
          )}
          <Button className="h-[30px] py-0" disabled={fieldsDisabled} onClick={addForward}>{t("advanced.add")}</Button>
        </div>
        <p className={`mt-1.5 ${hintText} text-ink-faint`}>{t(forwardHintKeys[newForwardKind])} {t("conn.forwardLoopbackOnly")}</p>
      </EditorSection>

      <EditorSection title={t("advanced.otherHeading")} meta={otherFields.length + pendingOthers.length === 0 ? undefined : t("advanced.count", { count: otherFields.length + pendingOthers.length })}>
        <ul className="flex flex-col">
          {otherFields.map((field) => {
            const warning = [
              field.dangerous === true ? t("host.dangerousField", { keyword: field.keyword }) : "",
              field.duplicate === true ? t("host.duplicateKeyword") : "",
            ].filter(Boolean).join(" ");
            return (
              <li key={fieldKey(field)} className="border-t border-line py-1 first:border-t-0">
                <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] items-center gap-2">
                  <span className={`truncate font-mono text-xs ${removed.includes(field.line) ? "text-ink-faint line-through" : "text-ink-muted"}`} title={field.keyword}>{field.keyword}</span>
                  <input
                    aria-label={field.keyword}
                    value={draftFor(field)}
                    disabled={fieldsDisabled || !field.editable || removed.includes(field.line)}
                    onChange={(event) => setDrafts({ ...drafts, [fieldKey(field)]: event.target.value })}
                    className={`${compactControl} font-mono`}
                  />
                  {removeButton(field)}
                </div>
                {warning === "" ? null : <p className="mt-1 text-xs text-notice-ink">{warning}</p>}
              </li>
            );
          })}
          {pendingOthers.map((addition, index) => (
            <li key={`new-directive-${index}`} className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] items-center gap-2 border-t border-line py-1.5 first:border-t-0">
              <span className="truncate font-mono text-xs text-ink-muted">{addition.keyword}</span>
              <span className="truncate font-mono text-xs text-ink">{(addition.values ?? []).join(" ")}<span className="ml-2 font-sans text-[11px] text-notice-ink">{t("advanced.pending")}</span></span>
              <RemoveToggle removed={false} disabled={fieldsDisabled} label={addition.keyword ?? ""} onToggle={() => setAdditions(additions.filter((candidate) => candidate !== addition))} />
            </li>
          ))}
        </ul>
        <div className="mt-2 grid gap-2 @sm/editor:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
          <input id="new-directive" aria-label={t("host.newDirective")} placeholder={t("advanced.keywordPlaceholder")} value={newKeyword} disabled={fieldsDisabled} onChange={(event) => setNewKeyword(event.target.value)} className={`${compactControl} font-mono`} />
          <input id="new-value" aria-label={t("host.newValue")} placeholder={t("advanced.valuePlaceholder")} value={newValue} disabled={fieldsDisabled} onChange={(event) => setNewValue(event.target.value)} className={`${compactControl} font-mono`} />
          <Button className="h-[30px] py-0" disabled={fieldsDisabled} onClick={addDirective}>{t("advanced.add")}</Button>
        </div>
      </EditorSection>

      {fieldDirty ? (
        <DraftSaveBar
          saveLabel={t("host.saveChanges")}
          saving={saving}
          saveDisabled={fieldsDisabled}
          discardDisabled={saving}
          onDiscard={discard}
          onSave={submitFieldEdits}
        />
      ) : null}
    </section>
  );
}

// RemoveToggle は、行を消す印を付ける（保存するまで消さない）ボタンと、その印を外す［戻す］である。
function RemoveToggle({ removed, disabled, label, onToggle }: { removed: boolean; disabled: boolean; label: string; onToggle: () => void }) {
  const t = useTranslate();
  if (removed) {
    return (
      <button type="button" aria-label={t("advanced.keepNamed", { keyword: label })} disabled={disabled} onClick={onToggle} className="h-[30px] shrink-0 rounded-md px-2 text-xs text-accent hover:bg-hover">
        {t("host.keep")}
      </button>
    );
  }
  const name = t("advanced.removeNamed", { keyword: label });
  return (
    <button
      type="button"
      aria-label={name}
      title={name}
      disabled={disabled}
      onClick={onToggle}
      className="inline-flex size-[30px] shrink-0 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-danger"
    >
      <Icon name="close" className="size-3.5" />
    </button>
  );
}

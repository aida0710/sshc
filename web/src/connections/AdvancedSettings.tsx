import { useCallback, useEffect, useMemo, useState } from "react";
import type { FieldEdit, FormField, HostDetail } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { AdvancedArea } from "../routing/connectionRoute";
import { control, hintText, narrowControl } from "../ui/form";
import { Button, Card, Notice, Row } from "../ui/surface";
import { formatValues, parseValues } from "../rules/rules";
import { identityKey } from "./connectionBrowser";
import { activateTabFromKeyboard } from "../ui/tabKeyboard";

type AdvancedSettingsProps = {
  detail: HostDetail;
  area: AdvancedArea;
  onAreaChange: (area: AdvancedArea) => void;
  onFieldEdits: (edits: FieldEdit[]) => void;
  onBlockRaw: (raw: string) => void;
  disabled: boolean;
  onDirtyChange: (dirty: boolean) => void;
};

function fieldKey(field: FormField): string {
  return `${field.line}-${field.keyword}`;
}

const tabs: { area: AdvancedArea; label: "host.tabJump" | "conn.portForwarding" | "conn.advancedDirectives" | "host.tabRaw" }[] = [
  { area: "Jump", label: "host.tabJump" },
  { area: "Forwards", label: "conn.portForwarding" },
  { area: "Directives", label: "conn.advancedDirectives" },
  { area: "Raw", label: "host.tabRaw" },
];
const tabAreas = tabs.map((tab) => tab.area);

export function AdvancedSettings({
  detail,
  area,
  onAreaChange,
  onFieldEdits,
  onBlockRaw,
  disabled,
  onDirtyChange,
}: AdvancedSettingsProps) {
  const t = useTranslate();
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [removed, setRemoved] = useState<number[]>([]);
  const [additions, setAdditions] = useState<FieldEdit[]>([]);
  const [newKeyword, setNewKeyword] = useState("");
  const [newValue, setNewValue] = useState("");
  const [newForwardKind, setNewForwardKind] = useState<"local" | "dynamic">("local");
  const [newListenPort, setNewListenPort] = useState("");
  const [newDestination, setNewDestination] = useState("");
  const [blockRaw, setBlockRaw] = useState(detail.form.raw);
  const [localError, setLocalError] = useState("");

  const resetKey = `${identityKey(detail.form.entry.identity)}\u0000${detail.file.contents}`;
  useEffect(() => {
    setDrafts({});
    setRemoved([]);
    setAdditions([]);
    setNewKeyword("");
    setNewValue("");
    setNewForwardKind("local");
    setNewListenPort("");
    setNewDestination("");
    setBlockRaw(detail.form.raw);
    setLocalError("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  const visibleFields = useMemo(
    () => detail.form.fields.filter((field) =>
      area === "Directives"
        ? (field.category === "advanced" || field.category === "basic") && !isForwardField(field)
        : area === "Forwards"
          ? isForwardField(field)
        : area === "Jump" && field.category === "jump",
    ),
    [area, detail.form.fields],
  );
  const fieldDirty = removed.length > 0 || additions.length > 0 || Object.entries(drafts).some(([key, value]) => {
    const field = detail.form.fields.find((candidate) => fieldKey(candidate) === key);
    return field !== undefined && value !== formatValues(field.values);
  });
  const rawDirty = blockRaw !== detail.form.raw;
  const dirty = fieldDirty || rawDirty;
  const fieldsDisabled = disabled || rawDirty;
  const rawDisabled = disabled || fieldDirty;

  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange]);

  const discard = useCallback(() => {
    setDrafts({});
    setRemoved([]);
    setAdditions([]);
    setNewKeyword("");
    setNewValue("");
    setNewForwardKind("local");
    setNewListenPort("");
    setNewDestination("");
    setBlockRaw(detail.form.raw);
    setLocalError("");
  }, [detail.form.raw]);

  function draftFor(field: FormField): string {
    return drafts[fieldKey(field)] ?? formatValues(field.values) ?? "";
  }

  function submitFieldEdits() {
    const edits: FieldEdit[] = [];
    try {
      for (const field of detail.form.fields) {
        if (removed.includes(field.line)) {
          edits.push({ action: "remove", line: field.line });
          continue;
        }
        const draft = drafts[fieldKey(field)];
        if (draft === undefined || draft === formatValues(field.values)) continue;
        edits.push({ action: "set", line: field.line, values: parseValues(draft) });
      }
      edits.push(...additions);
    } catch {
      setLocalError(t("host.unbalancedQuote"));
      return;
    }
    if (edits.length === 0) return;
    setLocalError("");
    onFieldEdits(edits);
  }

  function addDirective() {
    if (newKeyword === "") {
      setLocalError(t("host.needsKeyword"));
      return;
    }
    try {
      setAdditions([...additions, { action: "add", keyword: newKeyword, values: parseValues(newValue) }]);
    } catch {
      setLocalError(t("host.unbalancedQuote"));
      return;
    }
    setNewKeyword("");
    setNewValue("");
    setLocalError("");
  }

  function addForward() {
    if (!validPort(newListenPort)) {
      setLocalError(t("conn.forwardInvalidPort"));
      return;
    }
    if (newForwardKind === "local" && !validDestination(newDestination)) {
      setLocalError(t("conn.forwardInvalidDestination"));
      return;
    }
    setAdditions([...additions, {
      action: "add",
      keyword: newForwardKind === "local" ? "LocalForward" : "DynamicForward",
      values: newForwardKind === "local" ? [newListenPort, newDestination] : [newListenPort],
    }]);
    setNewListenPort("");
    setNewDestination("");
    setLocalError("");
  }

  return (
    <section aria-label={t("conn.advancedLabel")} className="flex flex-col gap-4">
      <label className="flex items-center gap-3 md:hidden">
        <span className="shrink-0 text-xs font-medium text-ink-muted">{t("conn.advancedViewLabel")}</span>
        <select
          aria-label={t("conn.advancedViews")}
          value={area}
          onChange={(event) => onAreaChange(event.currentTarget.value as AdvancedArea)}
          className={`${control} min-h-11 min-w-0 flex-1 py-2`}
        >
          {tabs.map((tab) => <option key={tab.area} value={tab.area}>{t(tab.label)}</option>)}
        </select>
      </label>

      <div role="tablist" aria-label={t("conn.advancedViews")} className="hidden border-b border-line md:flex">
        {tabs.map((tab, index) => (
          <button
            key={tab.area}
            id={`advanced-area-${tab.area.toLowerCase()}-tab`}
            type="button"
            role="tab"
            aria-selected={area === tab.area}
            aria-controls="advanced-settings-panel"
            tabIndex={area === tab.area ? 0 : -1}
            onClick={() => onAreaChange(tab.area)}
            onKeyDown={(event) => activateTabFromKeyboard(event, index, tabAreas, onAreaChange)}
            className={`min-h-10 border-b-2 px-5 py-2 text-sm transition-colors ${area === tab.area ? "border-accent font-medium text-ink" : "border-transparent text-ink-muted hover:bg-select-fill/50 hover:text-ink"}`}
          >
            {t(tab.label)}
          </button>
        ))}
      </div>

      {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}

      <div
        id="advanced-settings-panel"
        role="tabpanel"
        aria-labelledby={`advanced-area-${area.toLowerCase()}-tab`}
      >
        <div hidden={area === "Raw"} className="flex flex-col gap-3">
        {rawDirty ? <Notice>{t("conn.advancedRawBlocksFields")}</Notice> : null}
        {area === "Forwards" ? (
          <>
            <Notice>{t("conn.forwardLoopbackOnly")}</Notice>
            {visibleFields.length === 0 && additions.filter(isForwardAddition).length === 0
              ? <p className={hintText}>{t("conn.forwardNoneSaved")}</p>
              : <Card>
                {visibleFields.map((field) => (
                  <Row
                    key={fieldKey(field)}
                    label={field.keyword.toLowerCase() === "localforward" ? t("conn.forwardLocal") : t("conn.forwardDynamic")}
                    stackOnNarrow
                    action={(
                      <Button
                        className="px-2 py-1 text-xs"
                        disabled={fieldsDisabled}
                        onClick={() => setRemoved(
                          removed.includes(field.line)
                            ? removed.filter((line) => line !== field.line)
                            : [...removed, field.line],
                        )}
                      >
                        {removed.includes(field.line) ? t("host.keep") : t("host.remove")}
                      </Button>
                    )}
                  >
                    <input
                      aria-label={`${field.keyword} ${field.line}`}
                      value={draftFor(field)}
                      disabled={fieldsDisabled || !field.editable || removed.includes(field.line)}
                      onChange={(event) => setDrafts({ ...drafts, [fieldKey(field)]: event.target.value })}
                      className={control}
                    />
                  </Row>
                ))}
                {additions.filter(isForwardAddition).map((addition, index) => (
                  <Row
                    key={`new-forward-${index}`}
                    label={addition.keyword === "LocalForward" ? t("conn.forwardLocal") : t("conn.forwardDynamic")}
                    hint={t("conn.forwardPendingSave")}
                    stackOnNarrow
                    action={<Button className="px-2 py-1 text-xs" disabled={fieldsDisabled} onClick={() => setAdditions(additions.filter((candidate) => candidate !== addition))}>{t("host.remove")}</Button>}
                  >
                    <span className="w-full truncate font-mono text-sm">{(addition.values ?? []).join(" ")}</span>
                  </Row>
                ))}
              </Card>}
            <Card radius="md" className="grid gap-3 p-4 sm:grid-cols-2 sm:items-end 2xl:grid-cols-[10rem_9rem_minmax(12rem,1fr)_auto]">
              <label className="flex flex-col gap-1 text-xs text-ink-muted">
                {t("conn.forwardType")}
                <select value={newForwardKind} disabled={fieldsDisabled} onChange={(event) => setNewForwardKind(event.currentTarget.value as "local" | "dynamic")} className={control}>
                  <option value="local">{t("conn.forwardLocal")}</option>
                  <option value="dynamic">{t("conn.forwardDynamic")}</option>
                </select>
              </label>
              <label className="flex flex-col gap-1 text-xs text-ink-muted">
                {t("conn.forwardListenPort")}
                <input inputMode="numeric" value={newListenPort} disabled={fieldsDisabled} onChange={(event) => setNewListenPort(event.currentTarget.value)} className={control} />
              </label>
              {newForwardKind === "local" ? (
                <label className="flex flex-col gap-1 text-xs text-ink-muted sm:col-span-2 2xl:col-span-1">
                  {t("conn.forwardDestination")}
                  <input aria-label={t("conn.forwardDestination")} placeholder="127.0.0.1:5432" value={newDestination} disabled={fieldsDisabled} onChange={(event) => setNewDestination(event.currentTarget.value)} className={control} />
                </label>
              ) : null}
              <Button className={`${narrowControl} sm:col-span-2 2xl:col-span-1 ${newForwardKind === "dynamic" ? "2xl:col-start-4" : ""}`} disabled={fieldsDisabled} onClick={addForward}>{t("conn.forwardAdd")}</Button>
            </Card>
            <p className={hintText}>{t(newForwardKind === "local" ? "conn.forwardDestinationHint" : "conn.forwardDynamicHint")}</p>
          </>
        ) : visibleFields.length === 0 ? <p className={hintText}>{t("conn.advancedNoFields")}</p> : (
          <Card>
            {visibleFields.map((field) => (
              <Row
                key={fieldKey(field)}
                label={field.keyword}
                warning={[
                  field.dangerous === true ? t("host.dangerousField", { keyword: field.keyword }) : "",
                  field.duplicate === true ? t("host.duplicateKeyword") : "",
                ].filter(Boolean).join(" ") || undefined}
                action={
                  <Button
                    className="px-2 py-1 text-xs"
                    disabled={fieldsDisabled}
                    onClick={() => setRemoved(
                      removed.includes(field.line)
                        ? removed.filter((line) => line !== field.line)
                        : [...removed, field.line],
                    )}
                  >
                    {removed.includes(field.line) ? t("host.keep") : t("host.remove")}
                  </Button>
                }
              >
                <input
                  aria-label={field.keyword}
                  value={draftFor(field)}
                  disabled={fieldsDisabled || !field.editable || removed.includes(field.line)}
                  onChange={(event) => setDrafts({ ...drafts, [fieldKey(field)]: event.target.value })}
                  className={control}
                />
              </Row>
            ))}
          </Card>
        )}

        <Card hidden={area !== "Directives"} radius="md" className="flex flex-col gap-3 p-4">
          <label htmlFor="new-directive" className="text-xs text-ink-muted">{t("host.newDirective")}</label>
          <input
            id="new-directive"
            value={newKeyword}
            disabled={fieldsDisabled}
            onChange={(event) => setNewKeyword(event.target.value)}
            className={control}
          />
          <label htmlFor="new-value" className="text-xs text-ink-muted">{t("host.newValue")}</label>
          <input
            id="new-value"
            value={newValue}
            disabled={fieldsDisabled}
            onChange={(event) => setNewValue(event.target.value)}
            className={control}
          />
          <Button className={narrowControl} disabled={fieldsDisabled} onClick={addDirective}>
            {t("host.addDirective")}
          </Button>
        </Card>

        {fieldDirty ? <div className="flex items-center justify-end gap-2 border-t border-line py-3">
          <Button disabled={!fieldDirty} onClick={discard}>{t("conn.discardChanges")}</Button>
          <Button kind="primary" disabled={!fieldDirty || fieldsDisabled} onClick={submitFieldEdits}>
            {t("host.saveChanges")}
          </Button>
        </div> : null}
        </div>

        <div hidden={area !== "Raw"} className="flex flex-col gap-2">
        {fieldDirty ? <Notice>{t("conn.advancedFieldsBlockRaw")}</Notice> : null}
        <label htmlFor="block-raw" className="text-xs text-ink-muted">{t("host.blockText")}</label>
        <textarea
          id="block-raw"
          aria-label={t("host.blockText")}
          value={blockRaw}
          disabled={rawDisabled}
          onChange={(event) => setBlockRaw(event.target.value)}
          rows={16}
          spellCheck={false}
          className="min-h-80 rounded-lg border border-control-line bg-tree p-4 font-mono text-xs leading-5 text-ink focus:border-accent focus:outline-none"
        />
        {rawDirty ? <div className="flex items-center justify-end gap-2 border-t border-line py-3">
          <Button disabled={!rawDirty} onClick={discard}>{t("conn.discardChanges")}</Button>
          <Button kind="primary" disabled={!rawDirty || rawDisabled} onClick={() => onBlockRaw(blockRaw)}>
            {t("host.saveBlock")}
          </Button>
        </div> : null}
        </div>
      </div>
    </section>
  );
}

function isForwardField(field: FormField): boolean {
  const keyword = field.keyword.toLowerCase();
  return keyword === "localforward" || keyword === "dynamicforward";
}

function isForwardAddition(edit: FieldEdit): boolean {
  return edit.action === "add" && (edit.keyword === "LocalForward" || edit.keyword === "DynamicForward");
}

function validPort(value: string): boolean {
  const port = Number(value);
  return /^\d+$/.test(value) && Number.isInteger(port) && port > 0 && port <= 65535;
}

function validDestination(value: string): boolean {
  const bracketed = /^\[[^\]]+\]:(\d+)$/.exec(value);
  if (bracketed !== null) return validPort(bracketed[1] ?? "");
  const separator = value.lastIndexOf(":");
  if (separator <= 0 || value.slice(0, separator).includes(":")) return false;
  return validPort(value.slice(separator + 1));
}

import { useCallback, useEffect, useMemo, useState } from "react";
import type { FieldEdit, FormField, HostDetail } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { AdvancedArea } from "../routing/connectionRoute";
import { control, hintText, narrowControl } from "../ui/form";
import { Button, Card, Notice, Row } from "../ui/surface";
import { formatValues, parseValues } from "./values";

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
  const [blockRaw, setBlockRaw] = useState(detail.form.raw);
  const [localError, setLocalError] = useState("");

  const resetKey = `${detail.form.entry.identity.path}\u0000${detail.form.entry.identity.alias}\u0000${detail.file.contents}`;
  useEffect(() => {
    setDrafts({});
    setRemoved([]);
    setAdditions([]);
    setNewKeyword("");
    setNewValue("");
    setBlockRaw(detail.form.raw);
    setLocalError("");
    // resetKey is the complete server snapshot these line-number drafts belong to.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  const visibleFields = useMemo(
    () => detail.form.fields.filter((field) =>
      area === "Directives"
        ? field.category === "advanced" || field.category === "basic"
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
    setBlockRaw(detail.form.raw);
    setLocalError("");
  }, [detail.form.raw]);

  function draftFor(field: FormField): string {
    return drafts[fieldKey(field)] ?? formatValues(field.values);
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

  const tabs: { area: AdvancedArea; label: "host.tabJump" | "conn.advancedDirectives" | "host.tabRaw" }[] = [
    { area: "Jump", label: "host.tabJump" },
    { area: "Directives", label: "conn.advancedDirectives" },
    { area: "Raw", label: "host.tabRaw" },
  ];

  return (
    <section aria-label={t("conn.advancedLabel")} className="flex flex-col gap-3">
      <div role="tablist" aria-label={t("conn.advancedViews")} className="flex gap-1 border-b border-line">
        {tabs.map((tab) => (
          <button
            key={tab.area}
            type="button"
            role="tab"
            aria-selected={area === tab.area}
            onClick={() => onAreaChange(tab.area)}
            className={`px-3 py-2 text-sm ${area === tab.area ? "border-b-2 border-ink text-ink" : "text-ink-muted"}`}
          >
            {t(tab.label)}
          </button>
        ))}
      </div>

      {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}

      <div hidden={area === "Raw"} className="flex flex-col gap-3">
        {rawDirty ? <Notice>{t("conn.advancedRawBlocksFields")}</Notice> : null}
        {visibleFields.length === 0 ? <p className={hintText}>{t("conn.advancedNoFields")}</p> : (
          <Card>
            {visibleFields.map((field) => (
              <Row
                key={fieldKey(field)}
                label={field.keyword}
                warning={field.dangerous === true ? t("host.dangerousField", { keyword: field.keyword }) : undefined}
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

        <div hidden={area !== "Directives"} className="flex flex-col gap-2 rounded border border-line p-3">
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
        </div>

        <div className="sticky bottom-0 flex items-center justify-end gap-2 border-t border-line bg-canvas py-3">
          <Button disabled={!fieldDirty} onClick={discard}>{t("conn.discardChanges")}</Button>
          <Button kind="primary" disabled={!fieldDirty || fieldsDisabled} onClick={submitFieldEdits}>
            {t("host.saveChanges")}
          </Button>
        </div>
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
          className="rounded border border-control-line bg-canvas p-3 font-mono text-xs"
        />
        <div className="sticky bottom-0 flex items-center justify-end gap-2 border-t border-line bg-canvas py-3">
          <Button disabled={!rawDirty} onClick={discard}>{t("conn.discardChanges")}</Button>
          <Button kind="primary" disabled={!rawDirty || rawDisabled} onClick={() => onBlockRaw(blockRaw)}>
            {t("host.saveBlock")}
          </Button>
        </div>
      </div>
    </section>
  );
}

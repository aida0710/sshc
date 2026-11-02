import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { refreshPresetLibraries } from "../settings/presetLibrary";
import { Icon } from "../ui/icons";
import { PageSection } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Button, Notice } from "../ui/surface";
import { CustomKeyDialog, type CustomKey } from "./CustomKeyDialog";
import { paletteGroups } from "./keyBarCatalog";
import { insertKey, isPlaced, keyAt, moveKey, moveToOtherRow, removeKey, replaceKey, type KeyBarLayout, type KeySlot } from "./keyBarLayout";
import { saveKeyBarLayout, useKeyBarLayout, useKeyBarPresets } from "./keyBarPresets";
import { KeyBarPresetSection } from "./KeyBarPresetSection";
import { KeyBarPreview } from "./KeyBarPreview";
import { KeyFace, spokenKeyName } from "./KeyFace";
import type { KeyBarKey } from "./keySequences";

// Editing is a key the person is making, or the place of one being changed.
type Editing = { kind: "create" } | { kind: "edit"; slot: KeySlot };

// KeyBarConfig arranges the phone's key bar of a settings preset (the one being edited): a tap selects a key
// in the order shown, the buttons under it move it, and a tap on a key on offer puts it after the selected one.
// Every change is saved at once, in the key bar preset the settings preset chose, or in a new one when it
// chose the default layout.
export function KeyBarConfig({ presetId }: { presetId?: string } = {}) {
  const t = useTranslate();
  const library = useKeyBarPresets(presetId);
  const layout = useKeyBarLayout(presetId);
  const [selected, setSelected] = useState<KeySlot | null>(null);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [error, setError] = useState("");
  const unavailable = library.loading || library.busy || library.error;
  const selectedKey = selected === null ? undefined : keyAt(layout, selected);
  const slot = selectedKey === undefined ? null : selected;

  // Another key bar preset is another order: what was selected in the last one means nothing in it.
  useEffect(() => { setSelected(null); }, [library.selected, presetId]);

  async function save(next: KeyBarLayout, nextSelected: KeySlot | null) {
    setError("");
    try {
      await saveKeyBarLayout(next, t("keyBar.newPresetName"), presetId);
      setSelected(nextSelected);
    } catch { setError(t("keyBar.saveFailed")); }
  }

  function place(key: KeyBarKey) {
    const placed = insertKey(layout, slot, key);
    if (placed === null) { setError(t("keyBar.rowFull")); return; }
    void save(placed.layout, placed.slot);
  }

  function keepCustomKey(key: CustomKey) {
    const current = editing;
    setEditing(null);
    if (current?.kind === "edit") void save(replaceKey(layout, current.slot, key), current.slot);
    else place(key);
  }

  const moved = slot === null ? null : moveToOtherRow(layout, slot);
  const editedKey = editing?.kind === "edit" ? keyAt(layout, editing.slot) : undefined;
  return (
    <div data-key-bar-editor className="flex flex-col gap-5">
      {library.error ? <Notice tone="danger" actions={<Button small onClick={() => { void refreshPresetLibraries(); }}>{t("keyBar.reload")}</Button>}>{t("keyBar.reloadRequired")}</Notice> : null}
      {library.loading ? <PanelState tone="loading" title={t("keyBar.loading")} /> : null}
      <KeyBarPresetSection presetId={presetId} layout={layout} onError={setError} />
      <PageSection heading={t("keyBar.orderHeading")} actions={<span className="text-xs text-ink-faint">{t("keyBar.orderHint")}</span>}>
        <KeyBarPreview layout={layout} selected={slot} onSelect={setSelected} disabled={unavailable} />
        <div className="mt-3 flex flex-wrap gap-2">
          <Button small disabled={unavailable || slot === null || slot.index === 0} onClick={() => {
            if (slot === null) return;
            const result = moveKey(layout, slot, -1);
            void save(result.layout, result.slot);
          }}><span className="inline-flex items-center gap-1"><Icon name="arrowLeft" className="size-3.5" />{t("keyBar.moveBack")}</span></Button>
          <Button small disabled={unavailable || slot === null || slot.index === layout[slot.row].length - 1} onClick={() => {
            if (slot === null) return;
            const result = moveKey(layout, slot, 1);
            void save(result.layout, result.slot);
          }}><span className="inline-flex items-center gap-1">{t("keyBar.moveForward")}<Icon name="arrowRight" className="size-3.5" /></span></Button>
          <Button small disabled={unavailable || moved === null} onClick={() => { if (moved !== null) void save(moved.layout, moved.slot); }}>
            {slot?.row === "moreKeys" ? t("keyBar.toKeys") : t("keyBar.toMore")}
          </Button>
          {selectedKey !== undefined && selectedKey.key === undefined && slot !== null ? (
            <Button small disabled={unavailable} onClick={() => setEditing({ kind: "edit", slot })}>{t("keyBar.edit")}</Button>
          ) : null}
          <Button small disabled={unavailable || slot === null} onClick={() => { if (slot !== null) void save(removeKey(layout, slot), null); }}>
            {/* The small button sets its own text colour; the span gives removing the danger colour, as drawn. */}
            <span className="text-danger">{t("keyBar.remove")}</span>
          </Button>
        </div>
      </PageSection>
      <PageSection heading={t("keyBar.paletteHeading")}>
        <div className="flex flex-col gap-3">
          {paletteGroups.map((group) => (
            <div key={group.heading} role="group" aria-label={t(group.heading)} className="flex flex-col gap-1.5">
              <span className="text-[11.5px] text-ink-faint">{t(group.heading)}</span>
              <div className="flex flex-wrap gap-1.5">
                {group.keys.map((key) => (
                  <button key={key.key ?? key.text} type="button" disabled={unavailable || isPlaced(layout, key)}
                    aria-label={spokenKeyName(t, key)} onClick={() => place(key)}
                    className="flex h-9 min-w-10 items-center justify-center rounded-[8px] border border-line bg-card px-2 font-mono text-[12px] text-ink disabled:opacity-35">
                    <KeyFace barKey={key} iconClassName="size-[15px]" />
                  </button>
                ))}
              </div>
            </div>
          ))}
          <div role="group" aria-label={t("keyBar.groupCustom")} className="flex flex-col gap-1.5">
            <span className="text-[11.5px] text-ink-faint">{t("keyBar.groupCustom")}</span>
            <div>
              <button type="button" disabled={unavailable} onClick={() => setEditing({ kind: "create" })}
                className="flex h-9 items-center gap-1.5 rounded-[8px] border border-dashed border-line px-3 text-[12.5px] text-accent disabled:opacity-35">
                <Icon name="plus" className="size-3.5" />{t("keyBar.createCustom")}
              </button>
            </div>
          </div>
        </div>
      </PageSection>
      {editing === null ? null : (
        <CustomKeyDialog
          {...(editedKey === undefined ? {} : { initial: editedKey })}
          onSubmit={keepCustomKey}
          onCancel={() => setEditing(null)}
        />
      )}
      <div aria-live="polite">{error === "" ? null : <Notice tone="danger">{error}</Notice>}</div>
    </div>
  );
}

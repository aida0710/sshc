import { defaultShortcutPresetId, refreshPresets, savePresetBindings, selectPreset, updatePresets, useBindings, usePresets } from "./presets";
import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Button, Notice } from "../ui/surface";
import { FieldRow } from "../ui/FieldRow";
import { sizedControl } from "../ui/form";
import { PageSection } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { newIdentifier } from "../ui/randomIdentifier";
import { defaultBindings, shortcutActions, shortcutKey, type Bindings, type ShortcutAction } from "./bindings";
import { maxShortcutPresetNameLength, maxShortcutPresets } from "../rules/shortcuts.generated";
import { isImeComposing } from "../ui/imeComposition";

const actionLabelKeys: Record<ShortcutAction, MessageKey> = {
  palette: "shortcuts.palette",
  terminalSearch: "shortcuts.terminalSearch",
  copy: "shortcuts.copy",
  paste: "shortcuts.paste",
  previousPrompt: "shortcuts.previousPrompt",
  nextPrompt: "shortcuts.nextPrompt",
  nextSession: "shortcuts.nextSession",
  previousSession: "shortcuts.previousSession",
  home: "shortcuts.home",
  sftp: "shortcuts.sftp",
};

// presetId is the settings preset being edited: the shortcut preset chosen here is the one it uses.
export function KeyConfig({ presetId }: { presetId?: string } = {}) {
  const t = useTranslate();
  const bindings = useBindings(presetId);
  const library = usePresets(presetId);
  const [name, setName] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const unavailable = library.loading || library.busy || library.error;
  const selectedPreset = library.presets.find((preset) => preset.id === library.selected);
  const [recording, setRecording] = useState<ShortcutAction | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function save(next: Bindings) {
    setMessage("");
    setError("");
    try {
      await savePresetBindings(next, presetId);
      setRecording(null);
      setMessage(t("shortcuts.saved"));
    } catch { setError(t("shortcuts.saveFailed")); }
  }

  async function manage(action: "copy" | "rename" | "delete") {
    setError(""); setMessage("");
    try {
      const id = action === "copy" ? newIdentifier() : library.selected;
      const next = action === "copy"
        ? [...library.presets, { id, name: name.trim(), bindings }]
        : action === "rename" ? library.presets.map((p) => p.id === id ? { ...p, name: name.trim() } : p)
        : library.presets.filter((p) => p.id !== id);
      await updatePresets(next, action === "delete" ? defaultShortcutPresetId : id, presetId);
      setName(""); setMessage(t("shortcuts.saved"));
    } catch { setError(t("shortcuts.saveFailed")); }
  }

  return <div data-shortcut-editor className="flex flex-col gap-5">
    {library.error ? <Notice tone="danger" actions={<Button small onClick={() => { void refreshPresets(); }}>{t("shortcuts.reload")}</Button>}>{t("shortcuts.reloadRequired")}</Notice> : null}
    {library.loading ? <PanelState tone="loading" title={t("shortcuts.loading")} /> : null}
    <PageSection heading={t("shortcuts.presetsHeading")}>
      <FieldRow label={t("shortcuts.preset")} hint={library.selected === "default" ? t("shortcuts.defaultPresetHint") : undefined}
        actions={library.selected === "default" ? undefined : <Button kind="dangerText" disabled={unavailable} onClick={() => setConfirmingDelete(true)}>{t("shortcuts.delete")}</Button>}>
        <select className={sizedControl("w-full max-w-xs")} value={library.selected} disabled={unavailable}
          onChange={(event) => { setError(""); selectPreset(event.target.value, presetId).catch(() => setError(t("shortcuts.saveFailed"))); }}>
          <option value="default">{t("shortcuts.defaultPreset")}</option>
          {library.presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
        </select>
      </FieldRow>
      <FieldRow label={t("shortcuts.presetName")} interactiveChildren>
        <div className="flex flex-wrap items-center gap-2">
          <input aria-label={t("shortcuts.presetName")} className={sizedControl("w-full max-w-xs")} maxLength={maxShortcutPresetNameLength} value={name} disabled={unavailable} onChange={(event) => setName(event.target.value)} />
          <Button disabled={unavailable || !name.trim() || library.presets.length >= maxShortcutPresets} onClick={() => { void manage("copy"); }}>{t("shortcuts.duplicate")}</Button>
          <Button disabled={unavailable || !name.trim() || library.selected === "default"} onClick={() => { void manage("rename"); }}>{t("shortcuts.rename")}</Button>
        </div>
      </FieldRow>
    </PageSection>
    {confirmingDelete && selectedPreset !== undefined ? <ConfirmDialog
      id="shortcut-preset-delete-heading"
      heading={t("shortcuts.confirmDelete")}
      body={<p className="text-sm text-ink-muted">{t("shortcuts.deleteBody", { name: selectedPreset.name })}</p>}
      confirmLabel={t("shortcuts.deleteEverywhere")}
      cancelLabel={t("shortcuts.cancelDelete")}
      onCancel={() => setConfirmingDelete(false)}
      onConfirm={() => { setConfirmingDelete(false); void manage("delete"); }}
    /> : null}
    <PageSection heading={t("shortcuts.keysHeading")} hint={t("shortcuts.hint")}
      actions={<Button small disabled={unavailable} onClick={() => { void save(defaultBindings); }}>{t("shortcuts.reset")}</Button>}>
      <fieldset disabled={unavailable}>
        {shortcutActions.map((action) => <FieldRow key={action} label={t(actionLabelKeys[action])} interactiveChildren>
          {/* The key and its clear button stay together on one line. */}
          <div className="flex items-center gap-2">
            <button type="button" aria-label={t("shortcuts.assign", { action: t(actionLabelKeys[action]) })}
              className={`${sizedControl("w-full max-w-xs")} text-left`}
              onClick={() => { setRecording(action); setError(""); setMessage(""); }}
              onBlur={() => setRecording(null)}
              onKeyDown={(event) => {
                if (recording !== action || isImeComposing(event)) return;
                if (event.key === "Tab" && !event.ctrlKey && !event.altKey && !event.metaKey) { setRecording(null); return; }
                event.preventDefault();
                event.stopPropagation();
                if (event.key === "Escape") { setRecording(null); return; }
                if (event.repeat) return;
                const key = shortcutKey(event.nativeEvent);
                if (key === null) return;
                const conflict = shortcutActions.find((other) => other !== action && bindings[other].includes(key));
                if (conflict !== undefined) { setError(t("shortcuts.conflict", { action: t(actionLabelKeys[conflict]) })); return; }
                void save({ ...bindings, [action]: [key] });
              }}>
              {recording === action ? t("shortcuts.recording") : bindings[action].join(" / ") || t("shortcuts.unassigned")}
            </button>
            <Button small onClick={() => { void save({ ...bindings, [action]: [] }); }} disabled={bindings[action].length === 0}
              aria-label={t("shortcuts.clearAction", { action: t(actionLabelKeys[action]) })}>{t("shortcuts.clear")}</Button>
          </div>
        </FieldRow>)}
      </fieldset>
    </PageSection>
    <div aria-live="polite">{error ? <Notice tone="danger">{error}</Notice> : message ? <p role="status" className="text-sm text-ink-muted">{message}</p> : null}</div>
  </div>;
}

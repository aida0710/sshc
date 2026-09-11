import { refreshPresets, savePresetBindings, selectPreset, updatePresets, usePresets } from "./presets";
import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { defaultBindings, shortcutActions, shortcutKey, useBindings, type Bindings, type ShortcutAction } from "./bindings";

export function KeyConfig() {
  const t = useTranslate();
  const bindings = useBindings();
  const library = usePresets();
  const [name, setName] = useState("");
  const [deleteID, setDeleteID] = useState<string | null>(null);
  const unavailable = library.loading || library.busy || library.error;
  const [recording, setRecording] = useState<ShortcutAction | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function save(next: Bindings) {
    setMessage("");
    setError("");
    try {
      await savePresetBindings(next);
      setRecording(null);
      setMessage(t("shortcuts.saved"));
    } catch { setError(t("shortcuts.saveFailed")); }
  }

  async function manage(action: "copy" | "rename" | "delete") {
    setError(""); setMessage("");
    try {
      const id = action === "copy" ? crypto.randomUUID() : library.selected;
      const next = action === "copy"
        ? [...library.presets, { id, name: name.trim(), bindings }]
        : action === "rename" ? library.presets.map((p) => p.id === id ? { ...p, name: name.trim() } : p)
        : library.presets.filter((p) => p.id !== id);
      await updatePresets(next, action === "delete" ? "default" : id);
      setName(""); setMessage(t("shortcuts.saved"));
    } catch { setError(t("shortcuts.saveFailed")); }
  }

  return <div data-shortcut-editor>
    <p className="mb-4 text-sm text-ink-muted">{t("shortcuts.hint")}</p>
    <div className="mb-4 flex flex-wrap items-end gap-3">
      <label className="text-sm">{t("shortcuts.preset")}<select className="ml-2 rounded border border-line bg-control p-2" value={library.selected} disabled={unavailable}
        onChange={(event) => { try { selectPreset(event.target.value); } catch { setError(t("shortcuts.saveFailed")); } }}>
        <option value="default">{t("shortcuts.defaultPreset")}</option>
        {library.presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
      </select></label>
      <label className="text-sm">{t("shortcuts.presetName")}<input className="ml-2 rounded border border-line bg-control p-2" maxLength={80} value={name} onChange={(event) => setName(event.target.value)} /></label>
      <Button disabled={unavailable || !name.trim() || library.presets.length >= 64} onClick={() => { void manage("copy"); }}>{t("shortcuts.duplicate")}</Button>
      <Button disabled={unavailable || !name.trim() || library.selected === "default"} onClick={() => { void manage("rename"); }}>{t("shortcuts.rename")}</Button>
      <Button disabled={unavailable || library.selected === "default"} onClick={() => setDeleteID(library.selected)}>{t("shortcuts.delete")}</Button>
    </div>
    {deleteID === library.selected ? <div className="mb-4" role="group" aria-label={t("shortcuts.confirmDelete")}>
      <p>{t("shortcuts.confirmDelete")}</p>
      <Button disabled={unavailable} onClick={() => { setDeleteID(null); void manage("delete"); }}>{t("shortcuts.deleteEverywhere")}</Button>
      <Button onClick={() => setDeleteID(null)}>{t("shortcuts.cancelDelete")}</Button>
    </div> : null}
    {library.error ? <div role="alert"><p>{t("shortcuts.reloadRequired")}</p><Button onClick={() => { void refreshPresets(); }}>{t("shortcuts.reload")}</Button></div> : null}
    {library.loading ? <p role="status">{t("shortcuts.loading")}</p> : null}
    <fieldset disabled={unavailable}>
    <ul className="divide-y divide-line">
      {shortcutActions.map((action) => <li key={action} className="flex flex-wrap items-center gap-3 py-3">
        <span className="min-w-40 flex-1 text-sm font-medium text-ink">{t(`shortcuts.${action}`)}</span>
        <button type="button" aria-label={t("shortcuts.assign", { action: t(`shortcuts.${action}`) })}
          className="min-h-10 min-w-48 rounded border border-line bg-control px-3 py-2 text-sm text-ink focus-visible:outline-2 focus-visible:outline-accent"
          onClick={() => { setRecording(action); setError(""); setMessage(""); }}
          onBlur={() => setRecording(null)}
          onKeyDown={(event) => {
            if (recording !== action) return;
            if (event.key === "Tab" && !event.ctrlKey && !event.altKey && !event.metaKey) { setRecording(null); return; }
            event.preventDefault();
            event.stopPropagation();
            if (event.key === "Escape") { setRecording(null); return; }
            if (event.repeat || event.nativeEvent.isComposing) return;
            const key = shortcutKey(event.nativeEvent);
            if (key === null) return;
            const conflict = shortcutActions.find((other) => other !== action && bindings[other].includes(key));
            if (conflict !== undefined) { setError(t("shortcuts.conflict", { action: t(`shortcuts.${conflict}`) })); return; }
            void save({ ...bindings, [action]: [key] });
          }}>
          {recording === action ? t("shortcuts.recording") : bindings[action].join(" / ") || t("shortcuts.unassigned")}
        </button>
        <Button onClick={() => { void save({ ...bindings, [action]: [] }); }} disabled={bindings[action].length === 0}
          aria-label={t("shortcuts.clearAction", { action: t(`shortcuts.${action}`) })}>{t("shortcuts.clear")}</Button>
      </li>)}
    </ul>
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
      <div aria-live="polite">{error ? <p role="alert" className="text-sm text-danger">{error}</p> : message ? <p role="status" className="text-sm text-ink-muted">{message}</p> : null}</div>
      <Button onClick={() => { void save(defaultBindings); }}>{t("shortcuts.reset")}</Button>
    </div>
    </fieldset>
  </div>;
}

import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { defaultBindings, saveBindings, shortcutActions, shortcutKey, useBindings, type Bindings, type ShortcutAction } from "./bindings";

export function KeyConfig() {
  const t = useTranslate();
  const bindings = useBindings();
  const [recording, setRecording] = useState<ShortcutAction | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  function save(next: Bindings) {
    setMessage("");
    setError("");
    try {
      saveBindings(next);
      setRecording(null);
      setMessage(t("shortcuts.saved"));
    } catch { setError(t("shortcuts.saveFailed")); }
  }

  return <div data-shortcut-editor>
    <p className="mb-4 text-sm text-ink-muted">{t("shortcuts.hint")}</p>
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
            save({ ...bindings, [action]: [key] });
          }}>
          {recording === action ? t("shortcuts.recording") : bindings[action].join(" / ") || t("shortcuts.unassigned")}
        </button>
        <Button onClick={() => save({ ...bindings, [action]: [] })} disabled={bindings[action].length === 0}
          aria-label={t("shortcuts.clearAction", { action: t(`shortcuts.${action}`) })}>{t("shortcuts.clear")}</Button>
      </li>)}
    </ul>
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
      <div aria-live="polite">{error ? <p role="alert" className="text-sm text-danger">{error}</p> : message ? <p role="status" className="text-sm text-ink-muted">{message}</p> : null}</div>
      <Button onClick={() => save(defaultBindings)}>{t("shortcuts.reset")}</Button>
    </div>
  </div>;
}

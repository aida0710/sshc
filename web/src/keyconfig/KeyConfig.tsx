import { refreshPresets, savePresetBindings, shortcutPresetLibrary, useBindings, usePresets } from "./presets";
import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Button, Notice } from "../ui/surface";
import { FieldRow } from "../ui/FieldRow";
import { sizedControl } from "../ui/form";
import { PageSection } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { shortcutLabel } from "./shortcutLabel";
import { defaultBindings, shortcutActions, shortcutKey, type Bindings, type ShortcutAction } from "./bindings";
import { maxShortcutPresetNameLength, maxShortcutPresets } from "../rules/shortcuts.generated";
import { LibraryPresetChooser } from "../settings/LibraryPresetChooser";
import { isImeComposing } from "../ui/imeComposition";

const actionLabelKeys: Record<ShortcutAction, MessageKey> = {
  palette: "shortcuts.palette",
  terminalSearch: "shortcuts.terminalSearch",
  copy: "shortcuts.copy",
  paste: "shortcuts.paste",
  previousPrompt: "shortcuts.previousPrompt",
  nextPrompt: "shortcuts.nextPrompt",
  increaseFontSize: "shortcuts.increaseFontSize",
  decreaseFontSize: "shortcuts.decreaseFontSize",
  resetFontSize: "shortcuts.resetFontSize",
  quickCommands: "shortcuts.quickCommands",
  newSession: "shortcuts.newSession",
  closeSession: "shortcuts.closeSession",
  nextSession: "shortcuts.nextSession",
  previousSession: "shortcuts.previousSession",
  nextPane: "shortcuts.nextPane",
  previousPane: "shortcuts.previousPane",
  splitRight: "shortcuts.splitRight",
  splitBelow: "shortcuts.splitBelow",
  toggleNavigation: "shortcuts.toggleNavigation",
  home: "shortcuts.home",
  sftp: "shortcuts.sftp",
};

// presetId is the settings preset being edited: the shortcut preset chosen here is the one it uses.
export function KeyConfig({ presetId }: { presetId?: string } = {}) {
  const t = useTranslate();
  const bindings = useBindings(presetId);
  const library = usePresets(presetId);
  const unavailable = library.loading || library.busy || library.error;
  const [recording, setRecording] = useState<ShortcutAction | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function save(next: Bindings) {
    setMessage("");
    setError("");
    try {
      await savePresetBindings(next, t("shortcuts.newPresetName"), presetId);
      setRecording(null);
      setMessage(t("shortcuts.saved"));
    } catch { setError(t("shortcuts.saveFailed")); }
  }

  return <div data-shortcut-editor className="flex flex-col gap-5">
    {library.error ? <Notice tone="danger" actions={<Button small onClick={() => { void refreshPresets(); }}>{t("shortcuts.reload")}</Button>}>{t("shortcuts.reloadRequired")}</Notice> : null}
    {library.loading ? <PanelState tone="loading" title={t("shortcuts.loading")} /> : null}
    <PageSection heading={t("shortcuts.preset")}>
      <LibraryPresetChooser
        library={shortcutPresetLibrary}
        settingsPresetId={presetId}
        labels={{
          name: t("shortcuts.preset"),
          defaultName: t("shortcuts.defaultPreset"),
          unsavedHint: (name) => t("shortcuts.unsavedPresetHint", { name }),
          reset: t("shortcuts.reset"),
          renameHeading: t("shortcuts.renameHeading"),
          deleteHeading: t("shortcuts.confirmDelete"),
          saveFailed: t("shortcuts.saveFailed"),
        }}
        limits={{ presets: maxShortcutPresets, nameLength: maxShortcutPresetNameLength }}
        copyNamed={(id, name) => ({ id, name, bindings })}
        onReset={() => save(defaultBindings)}
        onError={(failure) => { setMessage(""); setError(failure); }}
      />
      <p className="mb-1 mt-3 text-xs leading-[18px] text-ink-faint">{t("shortcuts.hint")}</p>
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
              {recording === action ? t("shortcuts.recording") : bindings[action].map(shortcutLabel).join(" / ") || t("shortcuts.unassigned")}
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

import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { newIdentifier } from "../ui/randomIdentifier";
import { defaultPresetId, type LibraryPreset, type PresetLibrary } from "./presetLibrary";

// LibraryPresetLabels are the words of one library: what its presets are called on screen (キーの割り当て,
// キーの並び) keeps them apart from the settings presets above them.
export type LibraryPresetLabels = {
  name: string;
  defaultName: string;
  // unsavedHint says what editing does to the defaults or a built-in preset, which cannot change themselves.
  unsavedHint: (name: string) => string;
  reset: string;
  renameHeading: string;
  deleteHeading: string;
  saveFailed: string;
};

type Naming = "copy" | "rename";

// LibraryPresetChooser chooses the preset of a library that a settings preset uses, and holds the actions that
// copy, rename, reset and delete the library's presets. The defaults and the built-in presets are not saved presets:
// they can be copied, not renamed, reset or deleted.
export function LibraryPresetChooser<Preset extends LibraryPreset>({ library, builtIn = [], settingsPresetId, labels, limits, copyNamed, onReset, onError }: {
  library: PresetLibrary<Preset>;
  builtIn?: readonly Preset[];
  settingsPresetId?: string | undefined;
  labels: LibraryPresetLabels;
  limits: { presets: number; nameLength: number };
  // copyNamed makes a new preset holding what is shown now.
  copyNamed: (id: string, name: string) => Preset;
  onReset: () => Promise<void>;
  onError: (message: string) => void;
}) {
  const t = useTranslate();
  const state = library.usePresets(settingsPresetId);
  const [naming, setNaming] = useState<Naming | null>(null);
  const [deleting, setDeleting] = useState(false);
  const unavailable = state.loading || state.busy || state.error;
  const chosen = state.presets.find((preset) => preset.id === state.selected);
  const chosenName = chosen?.name ?? builtIn.find((preset) => preset.id === state.selected)?.name ?? labels.defaultName;

  async function attempt(action: () => Promise<void>) {
    onError("");
    try { await action(); } catch { onError(labels.saveFailed); }
  }

  async function name(value: string) {
    setNaming(null);
    await attempt(async () => {
      if (naming === "rename") {
        await library.update(state.presets.map((preset) => preset.id === state.selected ? { ...preset, name: value } : preset), state.selected, settingsPresetId);
        return;
      }
      const id = newIdentifier();
      await library.update([...state.presets, copyNamed(id, value)], id, settingsPresetId);
    });
  }

  const actions: ActionMenuItem[] = [
    { label: t("presetLibrary.duplicate"), onSelect: () => setNaming("copy"), disabled: unavailable || state.presets.length >= limits.presets },
    { label: t("presetLibrary.rename"), onSelect: () => setNaming("rename"), disabled: unavailable || chosen === undefined },
    { label: labels.reset, onSelect: () => void attempt(onReset), disabled: unavailable || chosen === undefined },
    { label: t("presetLibrary.delete"), onSelect: () => setDeleting(true), tone: "danger", disabled: unavailable || chosen === undefined },
  ];

  return (
    <div>
      <div className="flex min-w-0 items-center gap-2">
        <select aria-label={labels.name} className={sizedControl("w-full min-w-0 md:max-w-xs")} value={state.selected} disabled={unavailable}
          onChange={(event) => void attempt(() => library.select(event.target.value, settingsPresetId))}>
          <option value={defaultPresetId}>{labels.defaultName}</option>
          {builtIn.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
          {state.presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
        </select>
        <ActionMenu label={t("presetLibrary.actions", { name: chosenName })} items={actions}
          triggerClassName="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink md:size-7" />
      </div>
      {chosen === undefined ? <p className="mt-1.5 text-xs leading-[18px] text-ink-faint">{labels.unsavedHint(chosenName)}</p> : null}
      {naming === null ? null : (
        <InputDialog
          id="library-preset-name"
          heading={naming === "copy" ? t("presetLibrary.duplicateHeading", { name: chosenName }) : labels.renameHeading}
          label={t("presetLibrary.name")}
          initialValue={naming === "rename" ? chosenName : ""}
          submitLabel={naming === "copy" ? t("presetLibrary.duplicate") : t("presetLibrary.rename")}
          cancelLabel={t("presetLibrary.cancel")}
          validate={(value) => value === "" ? t("presetLibrary.nameRequired")
            : [...value].length > limits.nameLength ? t("presetLibrary.nameTooLong", { max: String(limits.nameLength) }) : ""}
          onSubmit={name}
          onCancel={() => setNaming(null)}
        />
      )}
      {deleting && chosen !== undefined ? (
        <ConfirmDialog
          id="library-preset-delete"
          heading={labels.deleteHeading}
          body={<p className="text-sm text-ink-muted">{t("presetLibrary.deleteBody", { name: chosen.name })}</p>}
          confirmLabel={t("presetLibrary.deleteEverywhere")}
          cancelLabel={t("presetLibrary.cancel")}
          onCancel={() => setDeleting(false)}
          onConfirm={() => {
            setDeleting(false);
            void attempt(() => library.update(state.presets.filter((preset) => preset.id !== chosen.id), defaultPresetId, settingsPresetId));
          }}
        />
      ) : null}
    </div>
  );
}

import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { Button } from "../ui/surface";
import { newIdentifier } from "../ui/randomIdentifier";
import { standardPresetId, updateMachine, updatePresets, useSettings, type SettingsPreset } from "./settingsStore";

// The limits the engine keeps to (application.MaxPresets and MaxPresetNameLength).
const maxPresets = 16;
const maxPresetNameLength = 80;

type Naming = { kind: "create" } | { kind: "rename"; name: string };

// presetName is how a preset is named on screen: the standard one has no name of its own.
export function presetName(preset: SettingsPreset, standardName: string): string {
  return preset.id === standardPresetId ? standardName : preset.name ?? "";
}

// PresetPicker chooses the preset the settings pages edit, says whether this machine uses it (or offers to
// use it here), and holds the actions that add, rename and delete presets. The standard preset cannot be
// renamed or deleted. It sits above the list of settings pages, and at the top of each preset page on a
// phone, where that list is not shown (layout "bar").
export function PresetPicker({ editing, onEdit, layout }: {
  editing: string;
  onEdit: (id: string) => void;
  layout: "list" | "bar";
}) {
  const t = useTranslate();
  const { document } = useSettings();
  const [naming, setNaming] = useState<Naming | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [failure, setFailure] = useState("");
  const presets = document?.presets ?? [{ id: standardPresetId }];
  const inUse = document?.presetInUse ?? standardPresetId;
  const edited = presets.find((preset) => preset.id === editing) ?? presets[0] ?? { id: standardPresetId };
  const editedName = presetName(edited, t("settings.presetStandard"));
  const unavailable = document === null;

  async function name(value: string) {
    setFailure("");
    try {
      if (naming?.kind === "rename") {
        await updatePresets((current) => current.map((preset) => preset.id === edited.id ? { ...preset, name: value } : preset));
      } else {
        // A new preset starts as a copy of the one open, so only what should differ needs changing.
        const created: SettingsPreset = { ...structuredClone(edited), id: newIdentifier(), name: value };
        await updatePresets((current) => [...current, created]);
        onEdit(created.id);
      }
      setNaming(null);
    } catch {
      setFailure(t("settings.presetSaveFailed"));
    }
  }

  async function remove() {
    setDeleting(false);
    try {
      await updatePresets((current) => current.filter((preset) => preset.id !== edited.id));
      onEdit(standardPresetId);
    } catch {
      setFailure(t("settings.presetSaveFailed"));
    }
  }

  function useHere() {
    const machineId = document?.machineId ?? "";
    updateMachine(machineId, ({ presetId: _previous, ...machine }) =>
      edited.id === standardPresetId ? machine : { ...machine, presetId: edited.id })
      .catch(() => setFailure(t("settings.presetSaveFailed")));
  }

  const actions: ActionMenuItem[] = [
    { label: t("settings.presetCreate"), onSelect: () => { setFailure(""); setNaming({ kind: "create" }); }, disabled: unavailable || presets.length >= maxPresets },
    { label: t("settings.presetRename"), onSelect: () => { setFailure(""); setNaming({ kind: "rename", name: editedName }); }, disabled: unavailable || edited.id === standardPresetId },
    { label: t("settings.presetDelete"), onSelect: () => setDeleting(true), tone: "danger", disabled: unavailable || edited.id === standardPresetId },
  ];
  const select = (
    <select aria-label={t("settings.presetToEdit")} className={sizedControl("w-full min-w-0")} value={edited.id} disabled={unavailable}
      onChange={(event) => onEdit(event.target.value)}>
      {presets.map((preset) => <option key={preset.id} value={preset.id}>{presetName(preset, t("settings.presetStandard"))}</option>)}
    </select>
  );
  const status = edited.id === inUse
    ? <span className="min-w-0 truncate whitespace-nowrap rounded-full bg-accent/15 px-2 text-[11.5px] leading-5 text-accent">{t("settings.presetInUse")}</span>
    : <Button small disabled={unavailable} onClick={useHere}>{t("settings.presetUseHere")}</Button>;
  const menu = <ActionMenu label={t("settings.presetActions", { name: editedName })} items={actions}
    triggerClassName="flex size-9 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink md:size-6" />;

  return (
    <div data-preset-picker className={layout === "list"
      ? "mx-0.5 mb-1.5 flex flex-col gap-1.5 rounded-[10px] border border-hairline bg-surface-subtle p-2"
      : "mb-3.5 flex flex-col gap-2 rounded-[10px] border border-hairline bg-surface-subtle px-3 py-2.5"}>
      {layout === "list" ? <span className="px-0.5 text-[11.5px] text-ink-faint">{t("settings.presetToEdit")}</span> : null}
      {layout === "list" ? select : <div className="flex min-w-0 items-center gap-2">{select}{menu}</div>}
      <div className="flex min-w-0 items-center gap-1.5">
        {status}
        <span className="grow" />
        {layout === "list" ? menu : null}
      </div>
      {failure === "" || naming !== null ? null : <p role="alert" className="text-xs text-danger">{failure}</p>}
      {naming === null ? null : (
        <InputDialog
          id="preset-name"
          heading={naming.kind === "create" ? t("settings.presetCreateHeading", { name: editedName }) : t("settings.presetRenameHeading")}
          label={t("settings.presetNameLabel")}
          initialValue={naming.kind === "rename" ? naming.name : ""}
          submitLabel={naming.kind === "create" ? t("settings.presetCreate") : t("settings.presetRename")}
          cancelLabel={t("settings.presetCancel")}
          validate={(value) => value === "" ? t("settings.presetNameRequired")
            : [...value].length > maxPresetNameLength ? t("settings.presetNameTooLong", { max: maxPresetNameLength }) : ""}
          error={failure}
          onSubmit={name}
          onCancel={() => setNaming(null)}
        />
      )}
      {deleting ? (
        <ConfirmDialog
          id="preset-delete"
          heading={t("settings.presetDeleteHeading")}
          body={<p className="text-sm text-ink-muted">{t("settings.presetDeleteBody", { name: editedName })}</p>}
          confirmLabel={t("settings.presetDeleteEverywhere")}
          cancelLabel={t("settings.presetCancel")}
          onCancel={() => setDeleting(false)}
          onConfirm={() => void remove()}
        />
      ) : null}
    </div>
  );
}

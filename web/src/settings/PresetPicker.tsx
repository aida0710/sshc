import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { Button, Notice } from "../ui/surface";
import { newIdentifier } from "../ui/randomIdentifier";
import { useSettingsDraft } from "./settingsDraftContext";
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
// renamed or deleted. The actions act at once on the saved presets, so they wait while there are unsaved
// changes. It sits above the list of settings pages, and at the top of each preset page on a phone, where
// that list is not shown (layout "bar").
export function PresetPicker({ editing, onEdit, layout }: {
  editing: string;
  onEdit: (id: string) => void;
  layout: "list" | "bar";
}) {
  const t = useTranslate();
  const { document } = useSettings();
  const { dirty } = useSettingsDraft();
  const [naming, setNaming] = useState<Naming | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [failure, setFailure] = useState("");
  const presets = document?.presets ?? [{ id: standardPresetId }];
  const inUse = document?.presetInUse ?? standardPresetId;
  const edited = presets.find((preset) => preset.id === editing) ?? presets[0] ?? { id: standardPresetId };
  const editedName = presetName(edited, t("settings.presetStandard"));
  const unavailable = document === null;
  const blocked = unavailable || dirty;
  const blockedReason = dirty ? t("settings.saveOrDiscardFirst") : undefined;

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
    { label: t("settings.presetCreate"), onSelect: () => { setFailure(""); setNaming({ kind: "create" }); }, disabled: blocked || presets.length >= maxPresets, title: blockedReason },
    { label: t("settings.presetRename"), onSelect: () => { setFailure(""); setNaming({ kind: "rename", name: editedName }); }, disabled: blocked || edited.id === standardPresetId, title: blockedReason },
    { label: t("settings.presetDelete"), onSelect: () => setDeleting(true), tone: "danger", disabled: blocked || edited.id === standardPresetId, title: blockedReason },
  ];
  const select = (
    <select aria-label={t("settings.presetToEdit")} className={sizedControl("w-full min-w-0")} value={edited.id} disabled={unavailable}
      onChange={(event) => onEdit(event.target.value)}>
      {presets.map((preset) => <option key={preset.id} value={preset.id}>{presetName(preset, t("settings.presetStandard"))}</option>)}
    </select>
  );
  const status = edited.id === inUse
    ? <span className="min-w-0 truncate whitespace-nowrap rounded-full bg-accent/15 px-2 text-xs leading-5 text-ink">{t("settings.presetInUse")}</span>
    : <Button small disabled={blocked} title={blockedReason} onClick={useHere}>{t("settings.presetUseHere")}</Button>;
  const menu = <ActionMenu label={t("settings.presetActions", { name: editedName })} items={actions}
    triggerSize="compact" />;

  return (
    <div data-preset-picker className={layout === "list"
      ? "mx-0.5 mb-1.5 flex flex-col gap-1.5 rounded-inset border border-hairline bg-surface-subtle p-2"
      : "mt-3 flex flex-col gap-2 rounded-inset border border-hairline bg-surface-subtle px-3 py-2.5"}>
      <span className="px-0.5 text-xs text-ink-faint">{t("settings.presetToEdit")}</span>
      {layout === "list" ? select : <div className="flex min-w-0 items-center gap-2">{select}{menu}</div>}
      <div className="flex min-w-0 items-center gap-1.5">
        {status}
        <span className="grow" />
        {layout === "list" ? menu : null}
      </div>
      {failure === "" || naming !== null ? null : <Notice tone="danger" compact>{failure}</Notice>}
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

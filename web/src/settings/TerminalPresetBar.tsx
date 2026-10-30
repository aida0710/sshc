import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu } from "../ui/ActionMenu";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { Button } from "../ui/surface";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { standardPresetId, type TerminalPreset } from "./terminalPresets";

// The limits the engine keeps to (application.MaxTerminalPresets and MaxTerminalPresetNameLength).
const maxTerminalPresets = 16;
const maxTerminalPresetNameLength = 80;

type Naming = { kind: "create" } | { kind: "rename"; name: string };

// TerminalPresetBar sits under the Terminal heading: which preset the page edits, whether this browser uses
// it, and the actions that add, rename and delete presets. The standard preset cannot be renamed or deleted.
// On a phone the preset fills the first row beside a "…" that holds the actions, and whether this browser
// uses it goes on the row under it, so a long name is cut short rather than pushing the row apart.
export function TerminalPresetBar({ presets, editing, inUse, disabled, onOpen, onUseHere, onCreate, onRename, onDelete }: {
  presets: TerminalPreset[];
  editing: string;
  inUse: string;
  disabled: boolean;
  onOpen: (id: string) => void;
  onUseHere: () => void;
  onCreate: (name: string) => Promise<void>;
  onRename: (name: string) => Promise<void>;
  onDelete: () => Promise<void>;
}) {
  const t = useTranslate();
  const mobile = useMediaQuery(mobileViewportQuery);
  const [naming, setNaming] = useState<Naming | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [failure, setFailure] = useState("");
  const edited = presets.find((preset) => preset.id === editing);
  const editedName = edited?.name ?? t("terminal.presetStandard");

  async function name(value: string) {
    setFailure("");
    try {
      if (naming?.kind === "rename") await onRename(value);
      else await onCreate(value);
      setNaming(null);
    } catch {
      setFailure(t("terminal.presetSaveFailed"));
    }
  }

  const actions = [
    { label: t("terminal.presetCreate"), disabled: disabled || presets.length >= maxTerminalPresets, run: () => { setFailure(""); setNaming({ kind: "create" }); } },
    { label: t("terminal.presetRename"), disabled: disabled || edited === undefined, run: () => { setFailure(""); setNaming({ kind: "rename", name: editedName }); } },
    { label: t("terminal.presetDelete"), disabled: disabled || edited === undefined, run: () => setDeleting(true) },
  ];
  const presetSelect = (
    <select aria-label={mobile ? t("terminal.presetLabel") : undefined} className={sizedControl(mobile ? "min-w-0 flex-1 truncate" : "w-[min(20rem,100%)] min-w-[190px] truncate")}
      value={editing} disabled={disabled} onChange={(event) => onOpen(event.target.value)}>
      <option value={standardPresetId}>{t("terminal.presetStandard")}</option>
      {presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
    </select>
  );
  const usage = editing === inUse ? (
    <span className="rounded-full bg-accent/15 px-2 text-[11.5px] leading-5 text-accent">{t("terminal.presetInUse")}</span>
  ) : (
    <Button small disabled={disabled} onClick={onUseHere}>{t("terminal.presetUseHere")}</Button>
  );

  return (
    <div data-terminal-preset-bar className={`mb-3.5 rounded-[10px] border border-hairline bg-surface-subtle px-3 py-2.5 ${mobile ? "flex flex-col items-start gap-2" : "flex flex-wrap items-center gap-2"}`}>
      {mobile ? (
        <>
          <div className="flex w-full min-w-0 items-center gap-1">
            {presetSelect}
            <ActionMenu label={t("terminal.presetActions")} items={actions.map(({ label, disabled: off, run }) => ({ label, disabled: off, onSelect: run }))}
              triggerClassName="flex size-11 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-hover" />
          </div>
          {usage}
        </>
      ) : (
        <>
          <label className="flex min-w-0 items-center gap-2">
            <span className="shrink-0 text-[12.5px] text-ink-muted">{t("terminal.presetLabel")}</span>
            {presetSelect}
          </label>
          {usage}
          <span className="grow" />
          {actions.map(({ label, disabled: off, run }) => <Button key={label} small disabled={off} onClick={run}>{label}</Button>)}
        </>
      )}
      {naming === null ? null : (
        <InputDialog
          id="terminal-preset-name"
          heading={naming.kind === "create" ? t("terminal.presetCreateHeading", { name: editedName }) : t("terminal.presetRenameHeading")}
          label={t("terminal.presetNameLabel")}
          initialValue={naming.kind === "rename" ? naming.name : ""}
          submitLabel={naming.kind === "create" ? t("terminal.presetCreate") : t("terminal.presetRename")}
          cancelLabel={t("terminal.presetCancel")}
          validate={(value) => value === "" ? t("terminal.presetNameRequired")
            : [...value].length > maxTerminalPresetNameLength ? t("terminal.presetNameTooLong", { max: maxTerminalPresetNameLength }) : ""}
          error={failure}
          onSubmit={name}
          onCancel={() => setNaming(null)}
        />
      )}
      {deleting && edited !== undefined ? (
        <ConfirmDialog
          id="terminal-preset-delete"
          heading={t("terminal.presetDeleteHeading")}
          body={<p className="text-sm text-ink-muted">{t("terminal.presetDeleteBody", { name: edited.name })}</p>}
          confirmLabel={t("terminal.presetDeleteEverywhere")}
          cancelLabel={t("terminal.presetCancel")}
          onCancel={() => setDeleting(false)}
          onConfirm={() => { setDeleting(false); void onDelete(); }}
        />
      ) : null}
    </div>
  );
}

import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { Button } from "../ui/surface";
import { standardPresetId, type TerminalPreset } from "./terminalPresets";

// The limits the engine keeps to (application.MaxTerminalPresets and MaxTerminalPresetNameLength).
const maxTerminalPresets = 16;
const maxTerminalPresetNameLength = 80;

type Naming = { kind: "create" } | { kind: "rename"; name: string };

// TerminalPresetBar sits under the Terminal heading: which set of settings the page edits, whether this
// browser uses it, and the buttons that add, rename and delete sets. The standard set cannot be renamed or
// deleted.
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

  return (
    <div data-terminal-preset-bar className="mb-3.5 flex flex-wrap items-center gap-2 rounded-[10px] border border-hairline bg-surface-subtle px-3 py-2.5">
      <label className="flex min-w-0 items-center gap-2">
        <span className="text-[12.5px] text-ink-muted">{t("terminal.presetLabel")}</span>
        <select className={sizedControl("min-w-[190px]")} value={editing} disabled={disabled} onChange={(event) => onOpen(event.target.value)}>
          <option value={standardPresetId}>{t("terminal.presetStandard")}</option>
          {presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
        </select>
      </label>
      {editing === inUse ? (
        <span className="rounded-full bg-accent/15 px-2 text-[11.5px] leading-5 text-accent">{t("terminal.presetInUse")}</span>
      ) : (
        <Button small disabled={disabled} onClick={onUseHere}>{t("terminal.presetUseHere")}</Button>
      )}
      <span className="grow" />
      <Button small disabled={disabled || presets.length >= maxTerminalPresets} onClick={() => { setFailure(""); setNaming({ kind: "create" }); }}>
        {t("terminal.presetCreate")}
      </Button>
      <Button small disabled={disabled || edited === undefined} onClick={() => { setFailure(""); setNaming({ kind: "rename", name: editedName }); }}>
        {t("terminal.presetRename")}
      </Button>
      <Button small disabled={disabled || edited === undefined} onClick={() => setDeleting(true)}>
        {t("terminal.presetDelete")}
      </Button>
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

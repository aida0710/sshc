import { useEffect, useState } from "react";
import { useLanguage } from "../i18n/context";
import { ActionMenu } from "../ui/ActionMenu";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sizedControl } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { isImeComposing } from "../ui/imeComposition";
import { PanelState } from "../ui/PanelState";
import { Notice } from "../ui/surface";
import { presetName } from "./PresetPicker";
import { SettingsSection } from "./SettingsSection";
import { removeMachine, standardPresetId, updateMachine, useSettings, type SettingsDocument } from "./settingsStore";

type Machine = SettingsDocument["machines"][number];

// The ports an engine may listen on (handoff.EnginePort): above the privileged range.
const minimumPort = 1024;
const maximumPort = 65535;
// The limit the engine keeps to (application.MaxMachineNameLength).
const maxMachineNameLength = 80;
// operatingSystemNames are the names people know the engine's operating systems by (Go's GOOS values).
const operatingSystemNames: Record<string, string> = {
  windows: "Windows", darwin: "macOS", linux: "Linux", android: "Android", freebsd: "FreeBSD", openbsd: "OpenBSD",
};

function operatingSystemName(os: string | undefined): string {
  if (os === undefined) return "";
  return operatingSystemNames[os] ?? os;
}

// columns lays a machine's row out like the heading above it, where the screen is wide enough.
const columns = "md:grid md:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_112px_120px_28px] md:items-center md:gap-3";

// The machines that sync: which preset each uses, the port its engine listens on, and when it last started.
// Any machine's settings can be changed from here; its engine reads them when it next starts (the port) or at
// once (the preset).
export function MachinesSettingsSection({ showHeading }: { showHeading: boolean }) {
  const { t } = useLanguage();
  const { document, loadFailed } = useSettings();
  const [failure, setFailure] = useState("");
  const [renaming, setRenaming] = useState<Machine | null>(null);
  const [removing, setRemoving] = useState<Machine | null>(null);

  function change(id: string, update: Parameters<typeof updateMachine>[1]) {
    setFailure("");
    return updateMachine(id, update).catch(() => setFailure(t("settings.machineSaveFailed")));
  }

  return (
    <SettingsSection id="settings-machines" label={t("settings.machinesHeading")} icon="engine" showHeading={showHeading}>
      {document === null
        ? (loadFailed ? <Notice tone="danger">{t("settings.machineSaveFailed")}</Notice> : <PanelState tone="loading" title={t("settings.machinesLoading")} />)
        : (
          <div className="flex flex-col gap-3">
            {failure === "" ? null : <Notice tone="danger" compact>{failure}</Notice>}
            <div className="overflow-hidden rounded-[10px] border border-line">
              <div className={`hidden border-b border-line px-3 py-2 text-[11.5px] text-ink-faint ${columns}`} aria-hidden="true">
                <span>{t("settings.machineColumnName")}</span>
                <span>{t("settings.machineColumnPreset")}</span>
                <span>{t("settings.machineColumnPort")}</span>
                <span>{t("settings.machineColumnStarted")}</span>
                <span />
              </div>
              {document.machines.map((machine) => (
                <MachineRow
                  key={machine.id}
                  machine={machine}
                  document={document}
                  onChange={change}
                  onRename={() => setRenaming(machine)}
                  onRemove={() => setRemoving(machine)}
                />
              ))}
            </div>
            <p className="text-xs leading-[18px] text-ink-faint">{t("settings.machinePortHint")}</p>
          </div>
        )}
      {renaming === null ? null : (
        <InputDialog
          id="machine-name"
          heading={t("settings.machineRenameHeading")}
          label={t("settings.machineNameLabel")}
          initialValue={renaming.settings.name ?? ""}
          submitLabel={t("settings.machineRename")}
          cancelLabel={t("settings.presetCancel")}
          validate={(value) => value === "" ? t("settings.presetNameRequired")
            : [...value].length > maxMachineNameLength ? t("settings.presetNameTooLong", { max: maxMachineNameLength }) : ""}
          onSubmit={(value) => {
            const id = renaming.id;
            setRenaming(null);
            void change(id, (settings) => ({ ...settings, name: value }));
          }}
          onCancel={() => setRenaming(null)}
        />
      )}
      {removing === null ? null : (
        <ConfirmDialog
          id="machine-remove"
          heading={t("settings.machineRemoveHeading")}
          body={<p className="text-sm text-ink-muted">{t("settings.machineRemoveBody", { name: removing.settings.name ?? "" })}</p>}
          confirmLabel={t("settings.machineRemove")}
          cancelLabel={t("settings.presetCancel")}
          onCancel={() => setRemoving(null)}
          onConfirm={() => {
            const id = removing.id;
            setRemoving(null);
            removeMachine(id).catch(() => setFailure(t("settings.machineSaveFailed")));
          }}
        />
      )}
    </SettingsSection>
  );
}

function MachineRow({ machine, document, onChange, onRename, onRemove }: {
  machine: Machine;
  document: SettingsDocument;
  onChange: (id: string, update: Parameters<typeof updateMachine>[1]) => Promise<unknown>;
  onRename: () => void;
  onRemove: () => void;
}) {
  const { t, locale } = useLanguage();
  const self = machine.id === document.machineId;
  const name = machine.settings.name ?? "";
  const presetId = document.presets.some((preset) => preset.id === machine.settings.presetId) ? machine.settings.presetId ?? standardPresetId : standardPresetId;
  const facts = [operatingSystemName(machine.facts.os), machine.facts.version === undefined ? "" : `sshc ${machine.facts.version.replace(/^v/u, "")}`]
    .filter((fact) => fact !== "").join(" · ");
  const started = machine.facts.startedAt === undefined
    ? t("settings.machineNeverStarted")
    : new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(machine.facts.startedAt));
  // The Android app keeps its own port: the WebView keeps what a page stores per port, so another port would
  // lose the browser's registration.
  const portFixed = machine.facts.os === "android";

  return (
    <div className={`flex flex-col gap-2 border-t border-line px-3 py-2.5 first-of-type:border-t-0 ${columns} ${self ? "bg-select-fill" : ""}`}>
      <div className="min-w-0">
        <div className="flex items-center gap-1.5 text-[13.5px] font-medium text-ink">
          <span className="truncate">{name}</span>
          {self ? <span className="rounded-full bg-accent/15 px-2 text-[11.5px] font-normal leading-5 text-accent">{t("settings.thisMachine")}</span> : null}
        </div>
        {facts === "" ? null : <div className="text-xs text-ink-faint">{facts}</div>}
      </div>
      <select aria-label={t("settings.machineColumnPreset")} className={sizedControl("w-full")} value={presetId}
        onChange={(event) => {
          // The value is read now: the write runs later, when the control shows the saved value again.
          const chosen = event.target.value;
          void onChange(machine.id, ({ presetId: _previous, ...settings }) => chosen === standardPresetId ? settings : { ...settings, presetId: chosen });
        }}>
        {document.presets.map((preset) => <option key={preset.id} value={preset.id}>{presetName(preset, t("settings.presetStandard"))}</option>)}
      </select>
      <PortInput machine={machine} disabled={portFixed} onChange={onChange} />
      <span className="text-xs text-ink-muted">{started}</span>
      <ActionMenu label={t("settings.machineActions", { name })}
        triggerClassName="flex size-9 items-center justify-center self-end rounded-md text-ink-muted hover:bg-hover hover:text-ink md:size-7 md:self-auto"
        items={[
        { label: t("settings.machineRename"), onSelect: onRename },
        { label: t("settings.machineRemove"), onSelect: onRemove, tone: "danger", disabled: self },
      ]} />
    </div>
  );
}

// PortInput keeps what is typed until it is confirmed (Enter or leaving the field). An empty field is the
// machine's own fixed port; a number outside the range is put back.
function PortInput({ machine, disabled, onChange }: {
  machine: Machine;
  disabled: boolean;
  onChange: (id: string, update: Parameters<typeof updateMachine>[1]) => Promise<unknown>;
}) {
  const { t } = useLanguage();
  const saved = machine.settings.port === undefined ? "" : String(machine.settings.port);
  const [draft, setDraft] = useState(saved);
  useEffect(() => setDraft(saved), [saved]);

  function commit() {
    const trimmed = draft.trim();
    if (trimmed === saved) return;
    if (trimmed === "") {
      void onChange(machine.id, ({ port: _previous, ...settings }) => settings);
      return;
    }
    const port = Number(trimmed);
    if (!Number.isSafeInteger(port) || port < minimumPort || port > maximumPort) {
      setDraft(saved);
      return;
    }
    void onChange(machine.id, (settings) => ({ ...settings, port }));
  }

  return (
    <input
      type="number"
      inputMode="numeric"
      min={minimumPort}
      max={maximumPort}
      aria-label={t("settings.machineColumnPort")}
      className={`${sizedControl("w-full")} font-mono tabular-nums`}
      value={draft}
      placeholder={t("settings.machinePortAutomatic")}
      disabled={disabled}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (isImeComposing(event)) return;
        if (event.key === "Enter") event.currentTarget.blur();
        if (event.key === "Escape") {
          setDraft(saved);
          event.currentTarget.blur();
        }
      }}
    />
  );
}

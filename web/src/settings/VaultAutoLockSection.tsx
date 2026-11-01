import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { Notice } from "../ui/surface";
import { SettingsSection } from "./SettingsSection";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { updatePreset, useSettings, type SettingsPreset } from "./settingsStore";
import { useCommitOnChange } from "../ui/useCommitOnChange";

type AutoLockDraft = {
  mode: "idle" | "restart";
  value: string;
  unit: "minutes" | "hours";
};

// initialDraft is the engine's default: twelve hours without use (secret.IdleTimeout).
const initialDraft: AutoLockDraft = { mode: "idle", value: "12", unit: "hours" };
// The limits the engine keeps to (application.MinVaultAutoLockValue, MaxVaultAutoLockValue).
const minimumValue = 1;
const maximumValue = 999;

function draftOf(preset: SettingsPreset | undefined): AutoLockDraft {
  const autoLock = preset?.vaultAutoLock;
  return {
    mode: autoLock?.mode === "restart" ? "restart" : "idle",
    value: autoLock?.mode === "idle" ? String(autoLock.value ?? 12) : initialDraft.value,
    unit: autoLock?.mode === "idle" ? autoLock.unit ?? "hours" : initialDraft.unit,
  };
}

// How long the vault stays open unattended, in the preset being edited. Every machine that uses the preset
// follows it. The engine never locks a passwordless vault, so there the saved choice stays visible but cannot
// be changed until a master password is set.
export function VaultAutoLockSection({ presetId, showHeading, passwordless }: {
  presetId: string;
  showHeading: boolean;
  passwordless: boolean;
}) {
  const t = useTranslate();
  const settingsDocument = useSettings().document;
  const preset = settingsDocument?.presets.find((candidate) => candidate.id === presetId);
  const [draft, setDraft] = useState<AutoLockDraft>(initialDraft);
  const save = useAsyncOperation();
  const loaded = settingsDocument !== null;
  // edited is whether the draft has changes not saved yet; until it has, it follows the preset.
  const edited = useRef(false);
  // latestDraft is the draft as the last edit left it, for the save a typed number makes when it is left.
  const latestDraft = useRef(draft);
  const fields = useRef<HTMLDivElement>(null);
  useCommitOnChange(fields, () => {
    if (edited.current) void submit(latestDraft.current);
  });

  useEffect(() => {
    edited.current = false;
  }, [presetId]);

  useEffect(() => {
    if (edited.current) return;
    latestDraft.current = draftOf(preset);
    setDraft(latestDraft.current);
  }, [preset]);

  // type keeps a key typed in the number, saved when the field is left; choose saves a choice at once.
  function type(patch: Partial<AutoLockDraft>) {
    latestDraft.current = { ...latestDraft.current, ...patch };
    setDraft(latestDraft.current);
    edited.current = true;
    save.touch();
  }
  function choose(patch: Partial<AutoLockDraft>) {
    type(patch);
    void submit(latestDraft.current);
  }

  async function submit(submitted: AutoLockDraft) {
    const value = Number(submitted.value);
    if (submitted.mode === "idle" && (!Number.isSafeInteger(value) || value < minimumValue || value > maximumValue)) {
      save.fail(t("settings.vaultAutoLockOutOfRange"));
      return;
    }
    await save.run(async () => {
      await updatePreset(presetId, (current) => ({
        ...current,
        vaultAutoLock: submitted.mode === "restart" ? { mode: "restart" } : { mode: "idle", value, unit: submitted.unit },
      }));
      if (latestDraft.current === submitted) edited.current = false;
    }, { describe: () => t("settings.vaultAutoLockSaveFailed") });
  }

  const disabled = !loaded || passwordless;
  const notice: MessageKey | null = passwordless
    ? "settings.vaultAutoLockPasswordless"
    : draft.mode === "restart" ? "settings.vaultAutoLockRestartWarning" : null;
  return (
    <SettingsSection id="settings-vault-auto-lock" label={t("settings.vaultAutoLockLabel")} icon="secrets" showHeading={showHeading}>
      <div ref={fields}>
        <FieldRow label={t("settings.vaultAutoLockLabel")} hint={t("settings.vaultAutoLockHint")}>
          <select className={control} value={draft.mode} disabled={disabled}
            onChange={(event) => choose({ mode: event.target.value as "idle" | "restart" })}>
            <option value="idle">{t("settings.vaultAutoLockIdle")}</option>
            <option value="restart">{t("settings.vaultAutoLockRestart")}</option>
          </select>
        </FieldRow>
        {draft.mode === "idle" ? (
          <FieldRow label={t("settings.vaultAutoLockValue")} interactiveChildren>
            <div className="grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.65fr)] gap-2">
              <input type="number" min={minimumValue} max={maximumValue} aria-label={t("settings.vaultAutoLockValue")} className={control}
                value={draft.value} disabled={disabled} onChange={(event) => type({ value: event.target.value })} />
              <select aria-label={t("settings.vaultAutoLockUnit")} className={control} value={draft.unit} disabled={disabled}
                onChange={(event) => choose({ unit: event.target.value as "minutes" | "hours" })}>
                <option value="minutes">{t("settings.vaultAutoLockMinutes")}</option>
                <option value="hours">{t("settings.vaultAutoLockHours")}</option>
              </select>
            </div>
          </FieldRow>
        ) : null}
        {notice === null ? null : <div className="mt-2"><Notice>{t(notice)}</Notice></div>}
        {save.error !== "" ? <div className="mt-2"><Notice tone="danger">{save.error}</Notice></div>
          : !loaded ? <p role="status" className="mt-2 text-sm text-ink-muted">{t("settings.vaultAutoLockLoading")}</p>
          : null}
      </div>
    </SettingsSection>
  );
}

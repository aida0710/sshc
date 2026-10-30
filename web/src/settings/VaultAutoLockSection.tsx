import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { Button, Notice } from "../ui/surface";
import { ActionArea } from "../ui/ActionArea";
import { SettingsSection } from "./SettingsSection";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { updatePreset, useSettings, type SettingsPreset } from "./settingsStore";

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

  useEffect(() => {
    edited.current = false;
  }, [presetId]);

  useEffect(() => {
    if (!edited.current) setDraft(draftOf(preset));
  }, [preset]);

  function edit(patch: Partial<AutoLockDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
    edited.current = true;
    save.touch();
  }

  async function submit() {
    const value = Number(draft.value);
    if (draft.mode === "idle" && (!Number.isSafeInteger(value) || value < minimumValue || value > maximumValue)) {
      save.fail(t("settings.vaultAutoLockOutOfRange"));
      return;
    }
    await save.run(async () => {
      await updatePreset(presetId, (current) => ({
        ...current,
        vaultAutoLock: draft.mode === "restart" ? { mode: "restart" } : { mode: "idle", value, unit: draft.unit },
      }));
      edited.current = false;
    }, { describe: () => t("settings.vaultAutoLockSaveFailed") });
  }

  const disabled = !loaded || save.busy || passwordless;
  const notice: MessageKey | null = passwordless
    ? "settings.vaultAutoLockPasswordless"
    : draft.mode === "restart" ? "settings.vaultAutoLockRestartWarning" : null;
  return (
    <SettingsSection id="settings-vault-auto-lock" label={t("settings.vaultAutoLockLabel")} icon="secrets" showHeading={showHeading}>
      <div>
        <FieldRow label={t("settings.vaultAutoLockLabel")} hint={t("settings.vaultAutoLockHint")}>
          <select className={control} value={draft.mode} disabled={disabled}
            onChange={(event) => edit({ mode: event.target.value as "idle" | "restart" })}>
            <option value="idle">{t("settings.vaultAutoLockIdle")}</option>
            <option value="restart">{t("settings.vaultAutoLockRestart")}</option>
          </select>
        </FieldRow>
        {draft.mode === "idle" ? (
          <FieldRow label={t("settings.vaultAutoLockValue")} interactiveChildren>
            <div className="grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.65fr)] gap-2">
              <input type="number" min={minimumValue} max={maximumValue} aria-label={t("settings.vaultAutoLockValue")} className={control}
                value={draft.value} disabled={disabled} onChange={(event) => edit({ value: event.target.value })} />
              <select aria-label={t("settings.vaultAutoLockUnit")} className={control} value={draft.unit} disabled={disabled}
                onChange={(event) => edit({ unit: event.target.value as "minutes" | "hours" })}>
                <option value="minutes">{t("settings.vaultAutoLockMinutes")}</option>
                <option value="hours">{t("settings.vaultAutoLockHours")}</option>
              </select>
            </div>
          </FieldRow>
        ) : null}
        {notice === null ? null : <div className="mt-2"><Notice>{t(notice)}</Notice></div>}
        <ActionArea status={save.error === ""
          ? (!loaded
              ? <p role="status" className="text-sm text-ink-muted">{t("settings.vaultAutoLockLoading")}</p>
              : !save.saved ? undefined : <Notice tone="success">{t("settings.vaultAutoLockSaved")}</Notice>)
          : <Notice tone="danger">{save.error}</Notice>}
        >
          <Button kind="primary" busy={save.busy} disabled={!loaded || passwordless} onClick={() => void submit()}>
            {t("terminal.startSave")}
          </Button>
        </ActionArea>
      </div>
    </SettingsSection>
  );
}

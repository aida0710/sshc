import { useCallback, useState } from "react";
import type { SettingsApi } from "../api/settings";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Field, control } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { ActionArea, SettingsSection } from "./SettingsSection";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { SettingsLoadFailure } from "./SettingsLoadFailure";
import { useSettingsLoad } from "./useSettingsLoad";

export type EngineSettingsApi = Pick<SettingsApi, "engineSettings" | "setEngineSettings">;

type EngineDraft = {
  port: string;
  vaultAutoLockMode: "idle" | "restart";
  vaultAutoLockValue: string;
  vaultAutoLockUnit: "minutes" | "hours";
};

const initialDraft: EngineDraft = { port: "", vaultAutoLockMode: "idle", vaultAutoLockValue: "12", vaultAutoLockUnit: "hours" };

// The engine's listening port and how long the vault stays open unattended.
// The engine never locks a passwordless vault, so there the saved auto-lock
// choice stays visible but cannot be changed until a master password is set.
export function EngineSettingsSection({ api, showHeading, passwordless }: {
  api: EngineSettingsApi;
  showHeading: boolean;
  passwordless: boolean;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState<EngineDraft>(initialDraft);
  const save = useAsyncOperation();
  const loadSettings = useCallback(async () => {
    const settings = await api.engineSettings();
    setDraft({
      port: settings.port === undefined ? "" : String(settings.port),
      vaultAutoLockMode: settings.vaultAutoLock?.mode === "restart" ? "restart" : "idle",
      vaultAutoLockValue: settings.vaultAutoLock?.mode === "idle" ? String(settings.vaultAutoLock.value ?? 12) : initialDraft.vaultAutoLockValue,
      vaultAutoLockUnit: settings.vaultAutoLock?.mode === "idle" ? settings.vaultAutoLock.unit ?? "hours" : initialDraft.vaultAutoLockUnit,
    });
  }, [api]);
  const settingsLoad = useSettingsLoad(loadSettings);
  const loaded = settingsLoad.state === "loaded";

  function edit(patch: Partial<EngineDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
    save.touch();
  }

  async function submit() {
    const trimmed = draft.port.trim();
    const chosen = trimmed === "" ? undefined : Number(trimmed);
    if (chosen !== undefined && (!Number.isSafeInteger(chosen) || chosen < 1024 || chosen > 65535)) {
      save.fail(t("engine.portOutOfRange"));
      return;
    }
    const idleValue = Number(draft.vaultAutoLockValue);
    if (draft.vaultAutoLockMode === "idle" &&
        (!Number.isSafeInteger(idleValue) || idleValue < 1 || idleValue > 999)) {
      save.fail(t("engine.vaultAutoLockOutOfRange"));
      return;
    }
    await save.run(() => api.setEngineSettings({
      ...(chosen === undefined ? {} : { port: chosen }),
      vaultAutoLock: draft.vaultAutoLockMode === "restart"
        ? { mode: "restart" }
        : { mode: "idle", value: idleValue, unit: draft.vaultAutoLockUnit },
    }), { describe: () => t("engine.saveFailed") });
  }

  const disabled = !loaded || save.busy;
  const autoLockDisabled = disabled || passwordless;
  const autoLockNotice: MessageKey | null = passwordless
    ? "engine.vaultAutoLockPasswordless"
    : draft.vaultAutoLockMode === "restart" ? "engine.vaultAutoLockRestartWarning" : null;
  return (
    <SettingsSection id="settings-engine" label={t("engine.heading")} icon="settings" showHeading={showHeading}>
      <div className="max-w-2xl">
        <SettingsLoadFailure settingsLoad={settingsLoad} title={t("engine.loadFailed")} />
        <Field label={t("engine.portLabel")} hint={t("engine.portHint")}>
          <input
            type="number"
            min={1024}
            max={65535}
            className={control}
            value={draft.port}
            placeholder="54447"
            disabled={disabled}
            onChange={(event) => edit({ port: event.target.value })}
          />
        </Field>
        <div className="mt-5 border-t border-line pt-5">
          <Field label={t("engine.vaultAutoLockLabel")} hint={t("engine.vaultAutoLockHint")}>
            <select
              className={control}
              value={draft.vaultAutoLockMode}
              disabled={autoLockDisabled}
              onChange={(event) => edit({ vaultAutoLockMode: event.target.value as "idle" | "restart" })}
            >
              <option value="idle">{t("engine.vaultAutoLockIdle")}</option>
              <option value="restart">{t("engine.vaultAutoLockRestart")}</option>
            </select>
          </Field>
          {draft.vaultAutoLockMode === "idle" ? (
            <div className="mt-4 grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.65fr)] gap-3">
              <Field label={t("engine.vaultAutoLockValue")}>
                <input
                  type="number"
                  min={1}
                  max={999}
                  className={control}
                  value={draft.vaultAutoLockValue}
                  disabled={autoLockDisabled}
                  onChange={(event) => edit({ vaultAutoLockValue: event.target.value })}
                />
              </Field>
              <Field label={t("engine.vaultAutoLockUnit")}>
                <select
                  className={control}
                  value={draft.vaultAutoLockUnit}
                  disabled={autoLockDisabled}
                  onChange={(event) => edit({ vaultAutoLockUnit: event.target.value as "minutes" | "hours" })}
                >
                  <option value="minutes">{t("engine.vaultAutoLockMinutes")}</option>
                  <option value="hours">{t("engine.vaultAutoLockHours")}</option>
                </select>
              </Field>
            </div>
          ) : null}
          {autoLockNotice === null ? null : (
            <div className="mt-4">
              <Notice>{t(autoLockNotice)}</Notice>
            </div>
          )}
        </div>
        <ActionArea status={save.error === ""
          ? (settingsLoad.state === "loading"
              ? <p role="status" className="text-sm text-ink-muted">{t("engine.loading")}</p>
              : !loaded || !save.saved ? undefined : <Notice tone="notice">{t("engine.saved")}</Notice>)
          : <Notice tone="danger">{save.error}</Notice>}
        >
          <Button kind="primary" disabled={disabled} onClick={() => void submit()}>
            {t("terminal.startSave")}
          </Button>
        </ActionArea>
      </div>
    </SettingsSection>
  );
}

import { useEffect, useState } from "react";
import type { SettingsApi } from "../api/settings";
import { useTranslate } from "../i18n/context";
import { Field, control } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { ActionArea, SettingsSection } from "./SettingsSection";
import { useSaveState } from "./useSaveState";

export type EngineSettingsApi = Pick<SettingsApi, "engineSettings" | "setEngineSettings">;

type EngineDraft = {
  port: string;
  vaultAutoLockMode: "idle" | "restart";
  vaultAutoLockValue: string;
  vaultAutoLockUnit: "minutes" | "hours";
};

const initialDraft: EngineDraft = { port: "", vaultAutoLockMode: "idle", vaultAutoLockValue: "12", vaultAutoLockUnit: "hours" };

// The engine's listening port and how long the vault stays open unattended.
export function EngineSettingsSection({ api, showHeading }: { api: EngineSettingsApi; showHeading: boolean }) {
  const t = useTranslate();
  const [draft, setDraft] = useState<EngineDraft>(initialDraft);
  const [loaded, setLoaded] = useState(false);
  const save = useSaveState();

  useEffect(() => {
    let active = true;
    void api.engineSettings()
      .then((settings) => {
        if (!active) return;
        setDraft({
          port: settings.port === undefined ? "" : String(settings.port),
          vaultAutoLockMode: settings.vaultAutoLock?.mode === "restart" ? "restart" : "idle",
          vaultAutoLockValue: settings.vaultAutoLock?.mode === "idle" ? String(settings.vaultAutoLock.value ?? 12) : initialDraft.vaultAutoLockValue,
          vaultAutoLockUnit: settings.vaultAutoLock?.mode === "idle" ? settings.vaultAutoLock.unit ?? "hours" : initialDraft.vaultAutoLockUnit,
        });
      })
      .catch(() => undefined)
      .finally(() => { if (active) setLoaded(true); });
    return () => { active = false; };
  }, [api]);

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
    }), () => t("engine.saveFailed"));
  }

  const disabled = !loaded || save.busy;
  return (
    <SettingsSection id="settings-engine" label={t("engine.heading")} icon="settings" showHeading={showHeading}>
      <div className="max-w-2xl">
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
              disabled={disabled}
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
                  disabled={disabled}
                  onChange={(event) => edit({ vaultAutoLockValue: event.target.value })}
                />
              </Field>
              <Field label={t("engine.vaultAutoLockUnit")}>
                <select
                  className={control}
                  value={draft.vaultAutoLockUnit}
                  disabled={disabled}
                  onChange={(event) => edit({ vaultAutoLockUnit: event.target.value as "minutes" | "hours" })}
                >
                  <option value="minutes">{t("engine.vaultAutoLockMinutes")}</option>
                  <option value="hours">{t("engine.vaultAutoLockHours")}</option>
                </select>
              </Field>
            </div>
          ) : (
            <div className="mt-4">
              <Notice>{t("engine.vaultAutoLockRestartWarning")}</Notice>
            </div>
          )}
        </div>
        <ActionArea status={save.error === ""
          ? (!loaded
              ? <p role="status" className="text-sm text-ink-muted">{t("engine.loading")}</p>
              : !save.saved ? undefined : <Notice tone="notice">{t("engine.saved")}</Notice>)
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

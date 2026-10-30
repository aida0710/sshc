import { useState } from "react";
import { useLanguage } from "../i18n/context";
import { locales, localeLabelKeys, type Locale } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";
import { IntegerSetting } from "../ui/IntegerSetting";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField, control, sizedControl } from "../ui/form";
import { formatDuration } from "../ui/format";
import { PageSection } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Notice } from "../ui/surface";
import { SettingsSection } from "./SettingsSection";
import { updateGlobalSettings, useSettings, type GlobalSettings } from "./settingsStore";

const reconnectChoices: { value: string; label: MessageKey }[] = [
  { value: "", label: "terminal.reconnectDefault" },
  { value: "0", label: "terminal.reconnectNever" },
  { value: "1", label: "terminal.reconnectOnce" },
  { value: "2", label: "terminal.reconnectTwice" },
  { value: "3", label: "terminal.reconnectThrice" },
  { value: "5", label: "terminal.reconnectFive" },
];

const verbosityChoices: { value: string; label: MessageKey }[] = [
  { value: "0", label: "terminal.verbosityQuiet" },
  { value: "1", label: "terminal.verbosityBrief" },
  { value: "2", label: "terminal.verbosityDetailed" },
  { value: "3", label: "terminal.verbosityFull" },
];

const autoClearChoices = [0, 30, 300, 3600];
// Keep these bounds in step with the engine's persisted settings contract.
const maximumReconnectAttempts = 10;
// Checking the box chooses a useful bounded budget when no attempt count exists.
const defaultReconnectAttempts = 3;
// The background library's limit, in MiB (application.MinBackgroundCapacityMiB, MaxBackgroundCapacityMiB,
// DefaultBackgroundCapacityMiB).
const minimumBackgroundCapacity = 1;
const maximumBackgroundCapacity = 1024;
const defaultBackgroundCapacity = 16;
const mebibyte = 1 << 20;

// withoutEmpty drops the sections a change left empty, so the global settings hold only what was chosen.
function withoutEmpty(settings: GlobalSettings): GlobalSettings {
  return Object.fromEntries(Object.entries(settings).filter(([, value]) =>
    value !== undefined && value !== "" && !(typeof value === "object" && Object.keys(value).length === 0)));
}

// The global settings: the same on every machine, whichever preset it uses. Each control saves its change at
// once, as the SFTP settings do.
export function GeneralSettingsSection({ showHeading }: { showHeading: boolean }) {
  const { t, setLocale, followBrowser } = useLanguage();
  const { document, loadFailed } = useSettings();
  const [failed, setFailed] = useState(false);
  const global = document?.global ?? {};
  const terminal = global.terminal ?? {};
  const transfers = global.fileTransfers ?? {};

  function save(change: (settings: GlobalSettings) => GlobalSettings) {
    setFailed(false);
    updateGlobalSettings((settings) => withoutEmpty(change(settings))).catch(() => setFailed(true));
  }

  function chooseLanguage(value: string) {
    // The language on screen changes with the setting; LanguageProvider saves it.
    if (value === "") followBrowser();
    else setLocale(value as Locale);
  }

  if (document === null) {
    return (
      <SettingsSection id="settings-general" label={t("settings.generalHeading")} icon="global" showHeading={showHeading}>
        {loadFailed ? <Notice tone="danger">{t("settings.generalSaveFailed")}</Notice> : <PanelState tone="loading" title={t("terminal.settingsLoading")} />}
      </SettingsSection>
    );
  }
  return (
    <SettingsSection id="settings-general" label={t("settings.generalHeading")} icon="global" showHeading={showHeading}>
      {failed ? <div className="mb-5"><Notice tone="danger" compact>{t("settings.generalSaveFailed")}</Notice></div> : null}
      <div className="flex flex-col gap-3.5 md:gap-5">
        <PageSection heading={t("settings.generalDisplayHeading")}>
          <div>
            <FieldRow label={t("settings.languageLabel")}>
              <select className={control} value={global.language ?? ""} onChange={(event) => chooseLanguage(event.target.value)}>
                <option value="">{t("settings.languageBrowser")}</option>
                {locales.map((locale) => <option key={locale} value={locale}>{t(localeLabelKeys[locale])}</option>)}
              </select>
            </FieldRow>
          </div>
        </PageSection>
        <PageSection heading={t("settings.generalTerminalHeading")}>
          <div>
            <FieldRow label={t("terminal.reconnectLabel")} hint={t("terminal.reconnectHint")}>
              <select className={control} value={terminal.reconnect === undefined ? "" : String(terminal.reconnect)}
                onChange={(event) => {
                  // The value is read now: the write runs later, when the control shows the saved value again.
                  const chosen = event.target.value;
                  save((settings) => {
                    const { reconnect: _previous, ...rest } = settings.terminal ?? {};
                    return { ...settings, terminal: chosen === "" ? rest : { ...rest, reconnect: Number(chosen) } };
                  });
                }}>
                {reconnectChoices.map((choice) => <option key={choice.value} value={choice.value}>{t(choice.label)}</option>)}
              </select>
            </FieldRow>
            <FieldRow label={t("terminal.verbosityLabel")} hint={t("terminal.verbosityHint")}>
              <select className={control} value={String(terminal.verbosity ?? 0)}
                onChange={(event) => {
                  const chosen = event.target.value;
                  save((settings) => {
                    const { verbosity: _previous, ...rest } = settings.terminal ?? {};
                    return { ...settings, terminal: chosen === "0" ? rest : { ...rest, verbosity: Number(chosen) } };
                  });
                }}>
                {verbosityChoices.map((choice) => <option key={choice.value} value={choice.value}>{t(choice.label)}</option>)}
              </select>
            </FieldRow>
            <FieldRow label="OSC 52" hint={t("terminal.osc52SettingHint")} interactiveChildren>
              <CheckboxField label={t("settings.osc52Allow")} checked={terminal.osc52 ?? false} className="pt-1.5"
                onChange={(checked) => save((settings) => {
                  const { osc52: _previous, ...rest } = settings.terminal ?? {};
                  return { ...settings, terminal: checked ? { ...rest, osc52: true } : rest };
                })} />
            </FieldRow>
          </div>
        </PageSection>
        <PageSection heading={t("settings.generalTransfersHeading")}>
          <div>
            <FieldRow label={t("sftp.manager.autoClear")}>
              <select aria-label={t("sftp.manager.autoClear")} className={sizedControl("w-40")}
                value={autoClearChoices.includes(transfers.clearCompletedAfterSeconds ?? 0) ? transfers.clearCompletedAfterSeconds ?? 0 : 0}
                onChange={(event) => {
                  const seconds = Number(event.target.value);
                  save((settings) => {
                    const { clearCompletedAfterSeconds: _previous, ...rest } = settings.fileTransfers ?? {};
                    return { ...settings, fileTransfers: seconds === 0 ? rest : { ...rest, clearCompletedAfterSeconds: seconds } };
                  });
                }}>
                {autoClearChoices.map((choice) => (
                  <option key={choice} value={choice}>
                    {choice === 0 ? t("sftp.manager.autoClearOff") : t("sftp.manager.autoClearAfter", { duration: formatDuration(choice, t) })}
                  </option>
                ))}
              </select>
            </FieldRow>
            <FieldRow label={t("sftp.manager.connectionLoss")} interactiveChildren>
              <CheckboxField label={t("sftp.manager.autoReconnect")} checked={transfers.autoReconnect ?? false} className="pt-1.5"
                onChange={(checked) => save((settings) => {
                  const { autoReconnect: _previous, maxReconnectAttempts, ...rest } = settings.fileTransfers ?? {};
                  return {
                    ...settings,
                    fileTransfers: checked ? { ...rest, autoReconnect: true, maxReconnectAttempts: maxReconnectAttempts || defaultReconnectAttempts }
                      : { ...rest, ...(maxReconnectAttempts === undefined ? {} : { maxReconnectAttempts }) },
                  };
                })} />
            </FieldRow>
            <FieldRow label={t("sftp.manager.maxReconnectAttempts")}>
              <IntegerSetting label={t("sftp.manager.maxReconnectAttempts")} value={transfers.maxReconnectAttempts ?? 0}
                min={0} max={maximumReconnectAttempts} unit={t("sftp.manager.reconnectUnit")} widthClass="w-[80px]"
                onCommit={(value) => save((settings) => {
                  const { maxReconnectAttempts: _previous, ...rest } = settings.fileTransfers ?? {};
                  return { ...settings, fileTransfers: value === 0 ? rest : { ...rest, maxReconnectAttempts: value } };
                })} />
            </FieldRow>
          </div>
        </PageSection>
        <PageSection heading={t("settings.generalBackgroundsHeading")}>
          <div>
            <FieldRow label={t("settings.backgroundCapacityLabel")} hint={t("terminal.backgroundCapacityHint")}>
              <IntegerSetting label={t("settings.backgroundCapacityLabel")}
                value={(global.backgrounds?.capacityMiB ?? defaultBackgroundCapacity) * mebibyte}
                min={minimumBackgroundCapacity} max={maximumBackgroundCapacity} scale={mebibyte} unit="MiB" widthClass="w-[80px]"
                onCommit={(value) => save(({ backgrounds: _previous, ...settings }) =>
                  value / mebibyte === defaultBackgroundCapacity ? settings : { ...settings, backgrounds: { capacityMiB: value / mebibyte } })} />
            </FieldRow>
          </div>
        </PageSection>
      </div>
    </SettingsSection>
  );
}

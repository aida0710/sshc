import { useEffect, useState, type CSSProperties, type ReactNode } from "react";
import { failureCode } from "../api/client";
import {
  integrationsApi,
  type IntegrationsApi,
  type LocalShellProfile,
  type TerminalSettings,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import { appearanceOf, defaultTint } from "../terminal/appearance";
import { useBackgroundImage } from "../terminal/backgroundImage";
import { fontStack, fonts } from "../terminal/fonts";
import { palettes } from "../terminal/palettes";
import {
  browserNotificationPermission,
  agentSoundPresets,
  loadAgentSoundPreferences,
  playAgentSound,
  requestBrowserNotificationPermission,
  saveAgentSoundPreferences,
  showBrowserNotification,
  type AgentSoundPreferences,
  type BrowserNotificationPermission,
} from "../terminal/agentNotifications";
import type { TerminalSessionsState } from "../terminal/sessions";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { PasswordField } from "../ui/PasswordField";
import { CheckboxField, Field, control, hintText } from "../ui/form";
import { Icon, type IconName } from "../ui/icons";
import { PageHeader } from "../ui/page";
import { Button, Card, Notice } from "../ui/surface";
import {
  settingsPageMeta,
  type SettingsPage,
} from "./settingsRoute";

const mobileTouchTargets =
  "[&_button]:min-h-10 [&_a]:inline-flex [&_a]:min-h-10 [&_a]:items-center " +
  "md:[&_button]:min-h-0 md:[&_a]:min-h-0";

function SettingsSection({
  id,
  label,
  icon,
  showHeading = true,
  children,
}: {
  id: string;
  label: string;
  icon: IconName;
  showHeading?: boolean;
  children: ReactNode;
}) {
  return (
    <section id={id} aria-label={label} className="scroll-mt-6 border-b border-line last:border-b-0">
      <div className={showHeading
        ? "grid gap-5 px-4 py-6 sm:px-6 lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-8 lg:py-8"
        : "px-4 py-6 sm:px-6 lg:px-8 lg:py-8"}
      >
        {showHeading ? (
          <header className="flex items-center gap-3 self-start lg:sticky lg:top-6">
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-select-fill text-accent">
              <Icon name={icon} />
            </span>
            <h3 className="text-sm font-semibold text-ink">{label}</h3>
          </header>
        ) : null}
        <div className="min-w-0">{children}</div>
      </div>
    </section>
  );
}

function ActionArea({ children, status }: { children: ReactNode; status?: ReactNode }) {
  return (
    <div className="mt-6 flex min-h-12 flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
      <div className="min-w-0 flex-1">{status}</div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

function TerminalPreview({
  palette,
  font,
  background,
  tint,
  fontSize,
}: {
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
  fontSize: string;
}) {
  const backgroundURL = useBackgroundImage(background);
  const chosenSize = Number(fontSize);
  const previewSize = Number.isFinite(chosenSize) && chosenSize >= 8 && chosenSize <= 32 ? chosenSize : 13;
  const hasBackground = background !== "" && backgroundURL !== "";
  const style = {
    color: "var(--ui-term-fg)",
    fontFamily: fontStack(font),
    fontSize: previewSize,
    ...(hasBackground
      ? {
          "--ui-term-image": `url("${backgroundURL}")`,
          "--ui-term-tint": String(tint ?? defaultTint),
        }
      : {}),
  } as CSSProperties;

  return (
    <div className="overflow-hidden rounded-md border border-control-line bg-term-bg shadow-sm" aria-hidden="true">
      <div className="flex items-center gap-1.5 border-b border-white/10 bg-black/20 px-3 py-2">
        <span className="h-2 w-2 rounded-full bg-danger" />
        <span className="h-2 w-2 rounded-full bg-notice-ink" />
        <span className="h-2 w-2 rounded-full bg-live" />
        <span className="ml-2 font-mono text-[10px] text-white/60">sshc · preview</span>
      </div>
      <div
        data-terminal-preview=""
        {...(palette === "" ? {} : { "data-term-palette": palette })}
        {...(font === "" ? {} : { "data-term-font": font })}
        {...(hasBackground ? { "data-term-background": background } : {})}
        style={style}
        className="min-h-40 bg-term-bg p-4 font-mono leading-relaxed"
      >
        <p>
          <span style={{ color: "var(--ui-term-green)" }}>workspace</span>{" "}
          <span style={{ color: "var(--ui-term-blue)" }}>main</span>
        </p>
        <p>
          <span style={{ color: "var(--ui-term-cyan)" }}>$</span> ssh example.com
        </p>
        <p style={{ color: "var(--ui-term-bright-black)" }}>Connected to example.com</p>
        <p>
          <span style={{ color: "var(--ui-term-cyan)" }}>$</span>{" "}
          <span className="inline-block h-[1em] w-[0.5em] translate-y-[0.12em] bg-current" />
        </p>
      </div>
    </div>
  );
}

type SettingsPanelProps = {
  api?: IntegrationsApi;
  page?: SettingsPage | "All";
  onTerminalSettingsChange?: (settings: TerminalSettings) => void | Promise<void>;
  consoles?: Pick<TerminalSessionsState, "sessions" | "busy" | "closeAll">;
};

export function SettingsPanel({
  api = integrationsApi,
  page = "All",
  consoles,
  onTerminalSettingsChange,
}: SettingsPanelProps) {
  const t = useTranslate();
  const liveConsoles = (consoles?.sessions ?? []).filter((session) => session.exited === undefined).length;
  const [confirmingCloseAll, setConfirmingCloseAll] = useState(false);
  const [currentMaster, setCurrentMaster] = useState("");
  const [nextMaster, setNextMaster] = useState("");
  const [confirmMaster, setConfirmMaster] = useState("");
  const [masterBusy, setMasterBusy] = useState(false);
  const [masterError, setMasterError] = useState("");
  const [changed, setChanged] = useState("");
  const [startDirectory, setStartDirectory] = useState("");
  const [maxSessions, setMaxSessions] = useState("");
  const [scrollback, setScrollback] = useState("");
  const [browserScrollbackLines, setBrowserScrollbackLines] = useState("");
  const [fontSize, setFontSize] = useState("");
  const [port, setPort] = useState("");
  const [vaultAutoLockMode, setVaultAutoLockMode] = useState<"idle" | "restart">("idle");
  const [vaultAutoLockValue, setVaultAutoLockValue] = useState("12");
  const [vaultAutoLockUnit, setVaultAutoLockUnit] = useState<"minutes" | "hours">("hours");
  const [engineLoaded, setEngineLoaded] = useState(false);
  const [portBusy, setPortBusy] = useState(false);
  const [portError, setPortError] = useState("");
  const [portSaved, setPortSaved] = useState(false);
  const [verbosity, setVerbosity] = useState("0");
  const [reconnect, setReconnect] = useState("");
  const [palette, setPalette] = useState("");
  const [font, setFont] = useState("");
  const [background, setBackground] = useState("");
  const [tint, setTint] = useState<number | undefined>(undefined);
  const [copyOnSelect, setCopyOnSelect] = useState(true);
  const [rightClickPaste, setRightClickPaste] = useState(true);
  const [webgl, setWebgl] = useState(true);
  const [osc52, setOsc52] = useState(false);
  const [jisYenBackslash, setJisYenBackslash] = useState(false);
  const [localShellProfile, setLocalShellProfile] = useState("");
  const [localShellProfiles, setLocalShellProfiles] = useState<LocalShellProfile[]>([]);
  const [terminalBusy, setTerminalBusy] = useState(false);
  const [terminalLoaded, setTerminalLoaded] = useState(false);
  const [terminalError, setTerminalError] = useState("");
  const [terminalSaved, setTerminalSaved] = useState(false);
  const [notificationPermission, setNotificationPermission] = useState<BrowserNotificationPermission>(() =>
    browserNotificationPermission()
  );
  const [notificationBusy, setNotificationBusy] = useState(false);
  const [notificationError, setNotificationError] = useState("");
  const [notificationSounds, setNotificationSounds] = useState<AgentSoundPreferences>(() =>
    loadAgentSoundPreferences()
  );

  function changeNotificationSounds(next: AgentSoundPreferences) {
    setNotificationSounds(next);
    saveAgentSoundPreferences(next);
  }

  useEffect(() => {
    let active = true;
    void api.terminalSettings()
      .then((settings) => {
        if (!active) return;
        setStartDirectory(settings.startDirectory ?? "");
        setMaxSessions(settings.maxSessions === undefined ? "" : String(settings.maxSessions));
        setScrollback(settings.scrollbackBytes === undefined ? "" : String(settings.scrollbackBytes));
        setBrowserScrollbackLines(settings.browserScrollbackLines === undefined
          ? ""
          : String(settings.browserScrollbackLines));
        setFontSize(settings.fontSize === undefined ? "" : String(settings.fontSize));
        setVerbosity(String(settings.verbosity ?? 0));
        setReconnect(settings.reconnect === undefined ? "" : String(settings.reconnect));
        setPalette(settings.appearance?.palette ?? "");
        setFont(settings.appearance?.font ?? "");
        setBackground(settings.appearance?.background ?? "");
        setTint(settings.appearance?.backgroundTint);
        setCopyOnSelect(settings.copyOnSelect ?? true);
        setRightClickPaste(settings.rightClickPaste ?? true);
        setWebgl(settings.webgl ?? true);
        setOsc52(settings.osc52 ?? false);
        setJisYenBackslash(settings.jisYenBackslash ?? false);
        setLocalShellProfile(settings.localShellProfile ?? "");
      })
      .catch(() => undefined)
      .finally(() => {
        if (active) setTerminalLoaded(true);
      });
    void api
      .engineSettings()
      .then((settings) => {
        if (!active) return;
        setPort(settings.port === undefined ? "" : String(settings.port));
        if (settings.vaultAutoLock?.mode === "restart") {
          setVaultAutoLockMode("restart");
        } else if (settings.vaultAutoLock?.mode === "idle") {
          setVaultAutoLockMode("idle");
          setVaultAutoLockValue(String(settings.vaultAutoLock.value ?? 12));
          setVaultAutoLockUnit(settings.vaultAutoLock.unit ?? "hours");
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (active) setEngineLoaded(true);
      });
    if (api.localShellProfiles !== undefined) {
      void api.localShellProfiles()
        .then((answer) => {
          if (active) setLocalShellProfiles(answer.profiles);
        })
        .catch(() => undefined);
    }
    return () => {
      active = false;
    };
  }, [api]);

  useEffect(() => {
    const refreshPermission = () => setNotificationPermission(browserNotificationPermission());
    window.addEventListener("focus", refreshPermission);
    return () => window.removeEventListener("focus", refreshPermission);
  }, []);

  async function enableOrTestNotifications() {
    setNotificationBusy(true);
    setNotificationError("");
    try {
      const permission = await requestBrowserNotificationPermission();
      setNotificationPermission(permission);
      if (permission === "granted") {
        const delivered = showBrowserNotification({
          title: "sshc",
          body: t("terminal.browserNotificationsReady"),
          tag: "sshc-notification-permission",
        });
        if (!delivered) setNotificationError(t("terminal.browserNotificationsDeliveryFailed"));
      }
    } catch {
      setNotificationPermission(browserNotificationPermission());
      setNotificationError(t("terminal.browserNotificationsRequestFailed"));
    } finally {
      setNotificationBusy(false);
    }
  }

  async function saveEngine() {
    const trimmed = port.trim();
    const chosen = trimmed === "" ? undefined : Number(trimmed);
    if (chosen !== undefined && (!Number.isSafeInteger(chosen) || chosen < 1024 || chosen > 65535)) {
      setPortError(t("engine.portOutOfRange"));
      setPortSaved(false);
      return;
    }
    const idleValue = Number(vaultAutoLockValue);
    if (vaultAutoLockMode === "idle" &&
        (!Number.isSafeInteger(idleValue) || idleValue < 1 || idleValue > 999)) {
      setPortError(t("engine.vaultAutoLockOutOfRange"));
      setPortSaved(false);
      return;
    }
    setPortBusy(true);
    setPortError("");
    setPortSaved(false);
    try {
      await api.setEngineSettings({
        ...(chosen === undefined ? {} : { port: chosen }),
        vaultAutoLock: vaultAutoLockMode === "restart"
          ? { mode: "restart" }
          : { mode: "idle", value: idleValue, unit: vaultAutoLockUnit },
      });
      setPortSaved(true);
    } catch {
      setPortError(t("engine.saveFailed"));
    } finally {
      setPortBusy(false);
    }
  }

  async function saveTerminal() {
    const numberOr = (text: string): number | undefined => {
      const trimmed = text.trim();
      if (trimmed === "") return undefined;
      const value = Number(trimmed);
      return Number.isSafeInteger(value) ? value : Number.NaN;
    };
    const sessions = numberOr(maxSessions);
    const bytes = numberOr(scrollback);
    const lines = numberOr(browserScrollbackLines);
    const size = numberOr(fontSize);
    if (Number.isNaN(sessions) || Number.isNaN(bytes) || Number.isNaN(lines) || Number.isNaN(size)) {
      setTerminalError(t("terminal.limitsOutOfRange"));
      setTerminalSaved(false);
      return;
    }

    setTerminalBusy(true);
    setTerminalError("");
    setTerminalSaved(false);
    try {
      const directory = startDirectory.trim();
      const next: TerminalSettings = {
        ...(directory === "" ? {} : { startDirectory: directory }),
        ...(sessions === undefined ? {} : { maxSessions: sessions }),
        ...(bytes === undefined ? {} : { scrollbackBytes: bytes }),
        ...(lines === undefined ? {} : { browserScrollbackLines: lines }),
        ...(size === undefined ? {} : { fontSize: size }),
        ...(verbosity === "0" ? {} : { verbosity: Number(verbosity) }),
        ...(reconnect === "" ? {} : { reconnect: Number(reconnect) }),
        ...(copyOnSelect ? {} : { copyOnSelect: false }),
        ...(rightClickPaste ? {} : { rightClickPaste: false }),
        ...(webgl ? {} : { webgl: false }),
        ...(osc52 ? { osc52: true } : {}),
        ...(jisYenBackslash ? { jisYenBackslash: true } : {}),
        ...(localShellProfile === "" ? {} : { localShellProfile }),
        ...(appearanceOf({ palette, font, background, tint })),
      };
      await api.setTerminalSettings(next);
      // The PUT above is the durable operation. A live-console refresh is a
      // best-effort follow-up and must not turn a completed save into a false
      // failure message.
      try {
        await onTerminalSettingsChange?.(next);
      } catch {
        // The normal console poll will reconcile the view shortly.
      }
      setTerminalSaved(true);
    } catch (error) {
      const code = failureCode(error);
      setTerminalError(t(
        code === "start_directory_missing"
          ? "terminal.startMissing"
          : code === "start_directory_not_a_directory"
            ? "terminal.startNotADirectory"
            : code === "start_directory_unusable"
              ? "terminal.startUnusable"
              : code === "terminal_limits_out_of_range" || code === "invalid_request"
                ? "terminal.limitsOutOfRange"
                : "terminal.settingsSaveFailed",
      ));
    } finally {
      setTerminalBusy(false);
    }
  }

  function clearMasterFields() {
    setCurrentMaster("");
    setNextMaster("");
    setConfirmMaster("");
  }

  async function changeMaster() {
    setMasterBusy(true);
    setMasterError("");
    setChanged("");
    try {
      await api.changeMasterPassword(currentMaster, nextMaster);
      setChanged(t("secrets.changedMasterLocally"));
    } catch (caught) {
      setMasterError(
        failureCode(caught) === "wrong_passphrase" ? t("secrets.wrongCurrent") : t("secrets.changeFailed"),
      );
    } finally {
      clearMasterFields();
      setMasterBusy(false);
    }
  }

  const canChangeMaster = !masterBusy && currentMaster !== "" && nextMaster.length >= 12 &&
    nextMaster === confirmMaster;
  const pageTitle = page === "All" ? "settings.heading" : settingsPageMeta[page].label;
  const pageDescription = page === "All"
    ? "settings.pageDescription"
    : settingsPageMeta[page].description;

  return (
    <div className={`mx-auto flex w-full max-w-6xl flex-col gap-6 ${mobileTouchTargets}`}>
      <PageHeader title={t(pageTitle)} description={t(pageDescription)} />

      <Card radius="md">
        {page === "All" || page === "Engine" ? (
        <SettingsSection id="settings-engine" label={t("engine.heading")} icon="settings" showHeading={page === "All"}>
          <div className="max-w-2xl">
            <Field label={t("engine.portLabel")} hint={t("engine.portHint")}>
              <input
                type="number"
                min={1024}
                max={65535}
                className={control}
                value={port}
                placeholder="54447"
                disabled={!engineLoaded || portBusy}
                onChange={(event) => {
                  setPort(event.target.value);
                  setPortSaved(false);
                }}
              />
            </Field>
            <div className="mt-5 border-t border-line pt-5">
              <Field label={t("engine.vaultAutoLockLabel")} hint={t("engine.vaultAutoLockHint")}>
                <select
                  className={control}
                  value={vaultAutoLockMode}
                  disabled={!engineLoaded || portBusy}
                  onChange={(event) => {
                    setVaultAutoLockMode(event.target.value as "idle" | "restart");
                    setPortSaved(false);
                  }}
                >
                  <option value="idle">{t("engine.vaultAutoLockIdle")}</option>
                  <option value="restart">{t("engine.vaultAutoLockRestart")}</option>
                </select>
              </Field>
              {vaultAutoLockMode === "idle" ? (
                <div className="mt-4 grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.65fr)] gap-3">
                  <Field label={t("engine.vaultAutoLockValue")}>
                    <input
                      type="number"
                      min={1}
                      max={999}
                      className={control}
                      value={vaultAutoLockValue}
                      disabled={!engineLoaded || portBusy}
                      onChange={(event) => {
                        setVaultAutoLockValue(event.target.value);
                        setPortSaved(false);
                      }}
                    />
                  </Field>
                  <Field label={t("engine.vaultAutoLockUnit")}>
                    <select
                      className={control}
                      value={vaultAutoLockUnit}
                      disabled={!engineLoaded || portBusy}
                      onChange={(event) => {
                        setVaultAutoLockUnit(event.target.value as "minutes" | "hours");
                        setPortSaved(false);
                      }}
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
            <ActionArea status={portError === ""
              ? (!engineLoaded
                  ? <p role="status" className="text-sm text-ink-muted">{t("engine.loading")}</p>
                  : !portSaved ? undefined : <Notice tone="notice">{t("engine.saved")}</Notice>)
              : <Notice tone="danger">{portError}</Notice>}
            >
              <Button kind="primary" disabled={!engineLoaded || portBusy} onClick={() => void saveEngine()}>
                {t("terminal.startSave")}
              </Button>
            </ActionArea>
          </div>
        </SettingsSection>
        ) : null}

        {page === "All" || page === "Terminal" ? (
        <SettingsSection id="settings-terminal" label={t("terminal.settingsHeading")} icon="terminal" showHeading={page === "All"}>
          <fieldset
            disabled={!terminalLoaded || terminalBusy}
            className="min-w-0 border-0 p-0 disabled:opacity-70"
          >
          <p className="mb-5 max-w-3xl text-sm leading-6 text-ink-muted">
            {t("terminal.settingsStorageHint")}
          </p>
          <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_19rem]">
            <div className="grid min-w-0 gap-5 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <Field label={t("terminal.startLabel")} hint={t("terminal.startHint")}>
                  <input
                    type="text"
                    className={control}
                    value={startDirectory}
                    spellCheck={false}
                    placeholder="~/"
                    disabled={terminalBusy}
                    onChange={(event) => {
                      setStartDirectory(event.target.value);
                      setTerminalSaved(false);
                    }}
                  />
                </Field>
              </div>
              <div className="sm:col-span-2">
                <Field label={t("terminal.localShellProfileLabel")} hint={t("terminal.localShellProfileHint")}>
                  <select
                    className={control}
                    value={localShellProfile}
                    disabled={terminalBusy || localShellProfiles.length === 0}
                    onChange={(event) => {
                      setLocalShellProfile(event.target.value);
                      setTerminalSaved(false);
                    }}
                  >
                    <option value="">{t("terminal.localShellProfileSystem")}</option>
                    {localShellProfiles.filter((profile) => profile.id !== "default").map((profile) => (
                      <option key={profile.id} value={profile.id}>{profile.label} — {profile.path}</option>
                    ))}
                  </select>
                </Field>
              </div>
              <Field label={t("terminal.maxSessionsLabel")} hint={t("terminal.maxSessionsHint")}>
                <input
                  type="number"
                  min={1}
                  max={200}
                  className={control}
                  value={maxSessions}
                  placeholder="50"
                  disabled={terminalBusy}
                  onChange={(event) => {
                    setMaxSessions(event.target.value);
                    setTerminalSaved(false);
                  }}
                />
              </Field>
              <Field label={t("terminal.scrollbackLabel")} hint={t("terminal.scrollbackHint")}>
                <input
                  type="number"
                  min={16384}
                  max={4194304}
                  className={control}
                  value={scrollback}
                  placeholder="262144"
                  disabled={terminalBusy}
                  onChange={(event) => {
                    setScrollback(event.target.value);
                    setTerminalSaved(false);
                  }}
                />
              </Field>
              <Field label={t("terminal.browserScrollbackLabel")} hint={t("terminal.browserScrollbackHint")}>
                <input
                  type="number"
                  min={1000}
                  max={100000}
                  className={control}
                  value={browserScrollbackLines}
                  placeholder="5000"
                  disabled={terminalBusy}
                  onChange={(event) => {
                    setBrowserScrollbackLines(event.target.value);
                    setTerminalSaved(false);
                  }}
                />
              </Field>
              <Field label={t("terminal.verbosityLabel")} hint={t("terminal.verbosityHint")}>
                <select
                  className={control}
                  value={verbosity}
                  disabled={terminalBusy}
                  onChange={(event) => {
                    setVerbosity(event.target.value);
                    setTerminalSaved(false);
                  }}
                >
                  <option value="0">{t("terminal.verbosityQuiet")}</option>
                  <option value="1">{t("terminal.verbosityBrief")}</option>
                  <option value="2">{t("terminal.verbosityDetailed")}</option>
                  <option value="3">{t("terminal.verbosityFull")}</option>
                </select>
              </Field>
              <Field label={t("terminal.reconnectLabel")} hint={t("terminal.reconnectHint")}>
                <select
                  className={control}
                  value={reconnect}
                  disabled={terminalBusy}
                  onChange={(event) => {
                    setReconnect(event.target.value);
                    setTerminalSaved(false);
                  }}
                >
                  <option value="">{t("terminal.reconnectDefault")}</option>
                  <option value="0">{t("terminal.reconnectNever")}</option>
                  <option value="1">{t("terminal.reconnectOnce")}</option>
                  <option value="2">{t("terminal.reconnectTwice")}</option>
                  <option value="3">{t("terminal.reconnectThrice")}</option>
                  <option value="5">{t("terminal.reconnectFive")}</option>
                </select>
              </Field>
              <Field label={t("terminal.paletteLabel")} hint={t("terminal.paletteHint")}>
                <AppearancePicker
                  choices={palettes}
                  value={palette}
                  onChange={(next) => {
                    setPalette(next);
                    setTerminalSaved(false);
                  }}
                  unchosen={t("terminal.paletteFollowsTheme")}
                />
              </Field>
              <Field label={t("terminal.fontLabel")} hint={t("terminal.fontHint")}>
                <AppearancePicker
                  choices={fonts}
                  value={font}
                  onChange={(next) => {
                    setFont(next);
                    setTerminalSaved(false);
                  }}
                  unchosen={t("terminal.fontFollowsSystem")}
                />
              </Field>
              <Field label={t("terminal.fontSizeLabel")} hint={t("terminal.fontSizeHint")}>
                <input
                  type="number"
                  min={8}
                  max={32}
                  className={control}
                  value={fontSize}
                  placeholder="15 / 13"
                  disabled={terminalBusy}
                  onChange={(event) => {
                    setFontSize(event.target.value);
                    setTerminalSaved(false);
                  }}
                />
              </Field>
              <div className="sm:col-span-2">
                <Field label={t("terminal.backgroundLabel")} hint={t("terminal.backgroundHint")}>
                  <BackgroundPicker
                    value={background}
                    onChange={(next) => {
                      setBackground(next);
                      setTerminalSaved(false);
                    }}
                    tint={tint}
                    onTintChange={(next) => {
                      setTint(next);
                      setTerminalSaved(false);
                    }}
                    unchosen={t("terminal.backgroundNone")}
                  />
                </Field>
              </div>
              <div className="flex flex-col gap-1 rounded-lg bg-select-fill p-3">
                <CheckboxField
                  label={t("terminal.copyOnSelectLabel")}
                  checked={copyOnSelect}
                  disabled={terminalBusy}
                  onChange={(checked) => {
                    setCopyOnSelect(checked);
                    setTerminalSaved(false);
                  }}
                />
                <p className={hintText}>{t("terminal.copyOnSelectHint")}</p>
              </div>
              <div className="flex flex-col gap-1 rounded-lg bg-select-fill p-3">
                <CheckboxField
                  label={t("terminal.osc52DefaultLabel")}
                  checked={osc52}
                  disabled={terminalBusy}
                  onChange={(checked) => {
                    setOsc52(checked);
                    setTerminalSaved(false);
                  }}
                />
                <p className={hintText}>{t("terminal.osc52DefaultHint")}</p>
              </div>
              <div className="flex flex-col gap-1 rounded-lg bg-select-fill p-3">
                <CheckboxField
                  label={t("terminal.jisYenBackslashLabel")}
                  checked={jisYenBackslash}
                  disabled={terminalBusy}
                  onChange={(checked) => {
                    setJisYenBackslash(checked);
                    setTerminalSaved(false);
                  }}
                />
                <p className={hintText}>{t("terminal.jisYenBackslashHint")}</p>
              </div>
              <div className="flex flex-col gap-1 rounded-lg bg-select-fill p-3">
                <CheckboxField
                  label={t("terminal.rightClickPasteLabel")}
                  checked={rightClickPaste}
                  disabled={terminalBusy}
                  onChange={(checked) => {
                    setRightClickPaste(checked);
                    setTerminalSaved(false);
                  }}
                />
                <p className={hintText}>{t("terminal.rightClickPasteHint")}</p>
              </div>
              <div className="flex flex-col gap-1 rounded-lg bg-select-fill p-3">
                <CheckboxField
                  label={t("terminal.webglLabel")}
                  checked={webgl}
                  disabled={terminalBusy}
                  onChange={(checked) => {
                    setWebgl(checked);
                    setTerminalSaved(false);
                  }}
                />
                <p className={hintText}>{t("terminal.webglHint")}</p>
              </div>
            </div>
            <div className="self-start xl:sticky xl:top-6">
              <TerminalPreview
                palette={palette}
                font={font}
                background={background}
                tint={tint}
                fontSize={fontSize}
              />
            </div>
          </div>
          <ActionArea status={terminalError === ""
            ? (!terminalLoaded
                ? <p role="status" className="text-sm text-ink-muted">{t("terminal.settingsLoading")}</p>
                : !terminalSaved
                  ? undefined
                  : <p role="status" className="text-sm text-live">{t("terminal.settingsSaved")}</p>)
            : <Notice tone="danger">{terminalError}</Notice>}
          >
            <Button kind="primary" disabled={terminalBusy} onClick={() => void saveTerminal()}>
              {t("terminal.startSave")}
            </Button>
          </ActionArea>
          </fieldset>
        </SettingsSection>
        ) : null}

        {page === "All" || page === "Notifications" ? (
        <SettingsSection
          id="settings-notifications"
          label={t("terminal.browserNotificationsHeading")}
          icon="notification"
          showHeading={page === "All"}
        >
          <div className="grid max-w-2xl gap-5">
            <p className="text-sm leading-6 text-ink-muted">
              {t(
                notificationPermission === "granted"
                  ? "terminal.browserNotificationsGranted"
                  : notificationPermission === "denied"
                    ? "terminal.browserNotificationsDenied"
                    : notificationPermission === "unsupported"
                      ? "terminal.browserNotificationsUnsupported"
                      : "terminal.browserNotificationsDefault",
              )}
            </p>
            {notificationPermission === "default" || notificationPermission === "granted" ||
                notificationError !== "" ? (
              <ActionArea status={notificationError === ""
                ? (notificationPermission === "granted"
                    ? <p role="status" className="text-sm text-live">{t("terminal.browserNotificationsEnabled")}</p>
                    : undefined)
                : <Notice tone="danger">{notificationError}</Notice>}
              >
                {notificationPermission === "default" || notificationPermission === "granted" ? (
                  <Button
                    kind={notificationPermission === "default" ? "primary" : "secondary"}
                    disabled={notificationBusy}
                    onClick={() => void enableOrTestNotifications()}
                  >
                    {t(notificationPermission === "granted"
                      ? "terminal.browserNotificationsTest"
                      : "terminal.browserNotificationsEnable")}
                  </Button>
                ) : null}
              </ActionArea>
            ) : null}
            <div className="grid gap-4 border-t border-line pt-5 sm:grid-cols-2">
              <Field label={t("terminal.notificationAttentionSound")} hint={t("terminal.notificationSoundHint")}>
                <div className="flex gap-2">
                  <select
                    className={control}
                    value={notificationSounds.attention}
                    onChange={(event) => changeNotificationSounds({
                      ...notificationSounds,
                      attention: event.target.value as AgentSoundPreferences["attention"],
                    })}
                  >
                    {agentSoundPresets.map((preset) => (
                      <option key={preset} value={preset}>{t(`terminal.notificationSound.${preset}`)}</option>
                    ))}
                  </select>
                  <Button
                    aria-label={t("terminal.notificationPreviewAttention")}
                    onClick={() => playAgentSound("attention", notificationSounds)}
                  >
                    {t("terminal.notificationPreview")}
                  </Button>
                </div>
              </Field>
              <Field label={t("terminal.notificationCompletedSound")} hint={t("terminal.notificationSoundHint")}>
                <div className="flex gap-2">
                  <select
                    className={control}
                    value={notificationSounds.completed}
                    onChange={(event) => changeNotificationSounds({
                      ...notificationSounds,
                      completed: event.target.value as AgentSoundPreferences["completed"],
                    })}
                  >
                    {agentSoundPresets.map((preset) => (
                      <option key={preset} value={preset}>{t(`terminal.notificationSound.${preset}`)}</option>
                    ))}
                  </select>
                  <Button
                    aria-label={t("terminal.notificationPreviewCompleted")}
                    onClick={() => playAgentSound("completed", notificationSounds)}
                  >
                    {t("terminal.notificationPreview")}
                  </Button>
                </div>
              </Field>
              <Field
                label={t("terminal.notificationVolume")}
                hint={t("terminal.notificationVolumeHint", { volume: String(notificationSounds.volume) })}
              >
                <input
                  aria-label={t("terminal.notificationVolume")}
                  type="range"
                  min={0}
                  max={100}
                  step={5}
                  value={notificationSounds.volume}
                  onChange={(event) => changeNotificationSounds({
                    ...notificationSounds,
                    volume: Number(event.target.value),
                  })}
                  className="h-9 w-full accent-accent"
                />
              </Field>
            </div>
          </div>
        </SettingsSection>
        ) : null}

        {consoles === undefined || (page !== "All" && page !== "Connections") ? null : (
          <SettingsSection id="settings-connections" label={t("desktop.closeAllHeading")} icon="connections" showHeading={page === "All"}>
            <p className="max-w-2xl text-sm leading-6 text-ink-muted">{t("desktop.closeAllNote")}</p>
            <ActionArea status={(
              <p role="status" className="text-sm text-ink-muted">
                <span className="mr-2 inline-block h-2 w-2 rounded-full bg-live" />
                {t("desktop.openCount", { count: consoles.sessions.length })}
              </p>
            )}>
              <Button
                disabled={consoles.busy || consoles.sessions.length === 0}
                onClick={() =>
                  liveConsoles === 0 ? void consoles.closeAll() : setConfirmingCloseAll(true)
                }
              >
                {t("desktop.closeAll")}
              </Button>
            </ActionArea>
            {confirmingCloseAll ? (
              <ConfirmDialog
                id="close-all-consoles-heading"
                heading={t("desktop.closeAllHeading2", { count: String(liveConsoles) })}
                body={<p className="text-sm text-ink-muted">{t("desktop.closeAllBody")}</p>}
                confirmLabel={t("desktop.closeAllConfirm")}
                cancelLabel={t("desktop.closeAllCancel")}
                onCancel={() => setConfirmingCloseAll(false)}
                onConfirm={() => {
                  setConfirmingCloseAll(false);
                  void consoles.closeAll();
                }}
              />
            ) : null}
          </SettingsSection>
        )}

        {page === "All" || page === "Password" ? (
        <SettingsSection id="settings-password" label={t("secrets.changeHeading")} icon="secrets" showHeading={page === "All"}>
          <div className="max-w-2xl">
            <p className="mb-5 text-sm leading-6 text-ink-muted">{t("secrets.changeNote")}</p>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <PasswordField
                  label={t("secrets.currentMaster")}
                  value={currentMaster}
                  onChange={setCurrentMaster}
                  disabled={masterBusy}
                />
              </div>
              <PasswordField
                label={t("secrets.newMaster")}
                value={nextMaster}
                onChange={setNextMaster}
                disabled={masterBusy}
              />
              <PasswordField
                label={t("secrets.confirmMaster")}
                value={confirmMaster}
                onChange={setConfirmMaster}
                disabled={masterBusy}
              />
            </div>
            <ActionArea status={masterError === ""
              ? (changed === "" ? undefined : <p role="status" className="text-sm text-live">{changed}</p>)
              : <Notice tone="danger">{masterError}</Notice>}
            >
              <Button kind="primary" disabled={!canChangeMaster} onClick={() => void changeMaster()}>
                {t("secrets.change")}
              </Button>
            </ActionArea>
          </div>
        </SettingsSection>
        ) : null}
      </Card>
    </div>
  );
}

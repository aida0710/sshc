import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import type { LocalShellProfile, SettingsApi, TerminalSettings } from "../api/settings";
import { useTranslate } from "../i18n/context";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import { appearanceOf } from "../terminal/appearance";
import { fonts } from "../terminal/fonts";
import { palettes } from "../terminal/palettes";
import { CheckboxField, Field, control, hintText } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { ActionArea, SettingsSection } from "./SettingsSection";
import { TerminalPreview } from "./TerminalPreview";
import { useSaveState } from "./useSaveState";

export type TerminalSettingsApi = Pick<SettingsApi, "terminalSettings" | "setTerminalSettings" | "localShellProfiles">;

// Every field is kept as the text the user typed; numbers are read once, on
// save, so that a half-typed value never snaps back under the cursor.
type TerminalDraft = {
  startDirectory: string;
  localShellProfile: string;
  maxSessions: string;
  scrollback: string;
  browserScrollbackLines: string;
  fontSize: string;
  verbosity: string;
  reconnect: string;
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
  copyOnSelect: boolean;
  rightClickPaste: boolean;
  webgl: boolean;
  osc52: boolean;
  jisYenBackslash: boolean;
};

const initialDraft: TerminalDraft = {
  startDirectory: "", localShellProfile: "", maxSessions: "", scrollback: "", browserScrollbackLines: "", fontSize: "",
  verbosity: "0", reconnect: "", palette: "", font: "", background: "", tint: undefined,
  copyOnSelect: true, rightClickPaste: true, webgl: true, osc52: false, jisYenBackslash: false,
};

function draftOf(settings: TerminalSettings): TerminalDraft {
  const text = (value: number | undefined) => value === undefined ? "" : String(value);
  return {
    startDirectory: settings.startDirectory ?? "",
    localShellProfile: settings.localShellProfile ?? "",
    maxSessions: text(settings.maxSessions),
    scrollback: text(settings.scrollbackBytes),
    browserScrollbackLines: text(settings.browserScrollbackLines),
    fontSize: text(settings.fontSize),
    verbosity: String(settings.verbosity ?? 0),
    reconnect: text(settings.reconnect),
    palette: settings.appearance?.palette ?? "",
    font: settings.appearance?.font ?? "",
    background: settings.appearance?.background ?? "",
    tint: settings.appearance?.backgroundTint,
    copyOnSelect: settings.copyOnSelect ?? true,
    rightClickPaste: settings.rightClickPaste ?? true,
    webgl: settings.webgl ?? true,
    osc52: settings.osc52 ?? false,
    jisYenBackslash: settings.jisYenBackslash ?? false,
  };
}

function numberOr(text: string): number | undefined {
  const trimmed = text.trim();
  if (trimmed === "") return undefined;
  const value = Number(trimmed);
  return Number.isSafeInteger(value) ? value : Number.NaN;
}

// Turns the draft back into the settings document, leaving out every field
// that still has its default so the stored document says only what changed.
function settingsOf(draft: TerminalDraft): TerminalSettings | null {
  const sessions = numberOr(draft.maxSessions);
  const bytes = numberOr(draft.scrollback);
  const lines = numberOr(draft.browserScrollbackLines);
  const size = numberOr(draft.fontSize);
  if (Number.isNaN(sessions) || Number.isNaN(bytes) || Number.isNaN(lines) || Number.isNaN(size)) return null;
  const directory = draft.startDirectory.trim();
  return {
    ...(directory === "" ? {} : { startDirectory: directory }),
    ...(sessions === undefined ? {} : { maxSessions: sessions }),
    ...(bytes === undefined ? {} : { scrollbackBytes: bytes }),
    ...(lines === undefined ? {} : { browserScrollbackLines: lines }),
    ...(size === undefined ? {} : { fontSize: size }),
    ...(draft.verbosity === "0" ? {} : { verbosity: Number(draft.verbosity) }),
    ...(draft.reconnect === "" ? {} : { reconnect: Number(draft.reconnect) }),
    ...(draft.copyOnSelect ? {} : { copyOnSelect: false }),
    ...(draft.rightClickPaste ? {} : { rightClickPaste: false }),
    ...(draft.webgl ? {} : { webgl: false }),
    ...(draft.osc52 ? { osc52: true } : {}),
    ...(draft.jisYenBackslash ? { jisYenBackslash: true } : {}),
    ...(draft.localShellProfile === "" ? {} : { localShellProfile: draft.localShellProfile }),
    ...appearanceOf({ palette: draft.palette, font: draft.font, background: draft.background, tint: draft.tint }),
  };
}

function saveFailureKey(error: unknown) {
  const code = failureCode(error);
  return code === "start_directory_missing"
    ? "terminal.startMissing"
    : code === "start_directory_not_a_directory"
      ? "terminal.startNotADirectory"
      : code === "start_directory_unusable"
        ? "terminal.startUnusable"
        : code === "terminal_limits_out_of_range" || code === "invalid_request"
          ? "terminal.limitsOutOfRange"
          : "terminal.settingsSaveFailed";
}

// The embedded terminal: where it starts, its limits, how it looks and how
// it treats the clipboard and keyboard.
export function TerminalSettingsSection({ api, showHeading, onSettingsChange }: {
  api: TerminalSettingsApi;
  showHeading: boolean;
  onSettingsChange?: ((settings: TerminalSettings) => void | Promise<void>) | undefined;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState<TerminalDraft>(initialDraft);
  const [loaded, setLoaded] = useState(false);
  const [profiles, setProfiles] = useState<LocalShellProfile[]>([]);
  const save = useSaveState();

  useEffect(() => {
    let active = true;
    void api.terminalSettings()
      .then((settings) => { if (active) setDraft(draftOf(settings)); })
      .catch(() => undefined)
      .finally(() => { if (active) setLoaded(true); });
    if (api.localShellProfiles !== undefined) {
      void api.localShellProfiles()
        .then((answer) => { if (active) setProfiles(answer.profiles); })
        .catch(() => undefined);
    }
    return () => { active = false; };
  }, [api]);

  function edit(patch: Partial<TerminalDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
    save.touch();
  }

  async function submit() {
    const next = settingsOf(draft);
    if (next === null) {
      save.fail(t("terminal.limitsOutOfRange"));
      return;
    }
    await save.run(async () => {
      await api.setTerminalSettings(next);
      // The PUT above is the durable operation. A live-console refresh is a
      // best-effort follow-up and must not turn a completed save into a false
      // failure message.
      try {
        await onSettingsChange?.(next);
      } catch {
        // The normal console poll will reconcile the view shortly.
      }
    }, (error) => t(saveFailureKey(error)));
  }

  const busy = save.busy;
  return (
    <SettingsSection id="settings-terminal" label={t("terminal.settingsHeading")} icon="terminal" showHeading={showHeading}>
      <fieldset
        disabled={!loaded || busy}
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
                value={draft.startDirectory}
                spellCheck={false}
                placeholder="~/"
                disabled={busy}
                onChange={(event) => edit({ startDirectory: event.target.value })}
              />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field label={t("terminal.localShellProfileLabel")} hint={t("terminal.localShellProfileHint")}>
              <select
                className={control}
                value={draft.localShellProfile}
                disabled={busy || profiles.length === 0}
                onChange={(event) => edit({ localShellProfile: event.target.value })}
              >
                <option value="">{t("terminal.localShellProfileSystem")}</option>
                {profiles.filter((profile) => profile.id !== "default").map((profile) => (
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
              value={draft.maxSessions}
              placeholder="50"
              disabled={busy}
              onChange={(event) => edit({ maxSessions: event.target.value })}
            />
          </Field>
          <Field label={t("terminal.scrollbackLabel")} hint={t("terminal.scrollbackHint")}>
            <input
              type="number"
              min={16384}
              max={4194304}
              className={control}
              value={draft.scrollback}
              placeholder="262144"
              disabled={busy}
              onChange={(event) => edit({ scrollback: event.target.value })}
            />
          </Field>
          <Field label={t("terminal.browserScrollbackLabel")} hint={t("terminal.browserScrollbackHint")}>
            <input
              type="number"
              min={1000}
              max={100000}
              className={control}
              value={draft.browserScrollbackLines}
              placeholder="5000"
              disabled={busy}
              onChange={(event) => edit({ browserScrollbackLines: event.target.value })}
            />
          </Field>
          <Field label={t("terminal.verbosityLabel")} hint={t("terminal.verbosityHint")}>
            <select
              className={control}
              value={draft.verbosity}
              disabled={busy}
              onChange={(event) => edit({ verbosity: event.target.value })}
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
              value={draft.reconnect}
              disabled={busy}
              onChange={(event) => edit({ reconnect: event.target.value })}
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
              value={draft.palette}
              onChange={(next) => edit({ palette: next })}
              unchosen={t("terminal.paletteFollowsTheme")}
            />
          </Field>
          <Field label={t("terminal.fontLabel")} hint={t("terminal.fontHint")}>
            <AppearancePicker
              choices={fonts}
              value={draft.font}
              onChange={(next) => edit({ font: next })}
              unchosen={t("terminal.fontFollowsSystem")}
            />
          </Field>
          <Field label={t("terminal.fontSizeLabel")} hint={t("terminal.fontSizeHint")}>
            <input
              type="number"
              min={8}
              max={32}
              className={control}
              value={draft.fontSize}
              placeholder="15 / 13"
              disabled={busy}
              onChange={(event) => edit({ fontSize: event.target.value })}
            />
          </Field>
          <div className="sm:col-span-2">
            <Field label={t("terminal.backgroundLabel")} hint={t("terminal.backgroundHint")} interactiveChildren>
              <BackgroundPicker
                value={draft.background}
                onChange={(next) => edit({ background: next })}
                tint={draft.tint}
                onTintChange={(next) => edit({ tint: next })}
                unchosen={t("terminal.backgroundNone")}
              />
            </Field>
          </div>
          {([
            ["copyOnSelect", "terminal.copyOnSelectLabel", "terminal.copyOnSelectHint"],
            ["osc52", "terminal.osc52DefaultLabel", "terminal.osc52DefaultHint"],
            ["jisYenBackslash", "terminal.jisYenBackslashLabel", "terminal.jisYenBackslashHint"],
            ["rightClickPaste", "terminal.rightClickPasteLabel", "terminal.rightClickPasteHint"],
            ["webgl", "terminal.webglLabel", "terminal.webglHint"],
          ] as const).map(([key, label, hint]) => (
            <div key={key} className="flex flex-col gap-1 rounded-lg bg-select-fill p-3">
              <CheckboxField
                label={t(label)}
                checked={draft[key]}
                disabled={busy}
                onChange={(checked) => edit({ [key]: checked })}
              />
              <p className={hintText}>{t(hint)}</p>
            </div>
          ))}
        </div>
        <div className="self-start xl:sticky xl:top-6">
          <TerminalPreview
            palette={draft.palette}
            font={draft.font}
            background={draft.background}
            tint={draft.tint}
            fontSize={draft.fontSize}
          />
        </div>
      </div>
      <ActionArea status={save.error === ""
        ? (!loaded
            ? <p role="status" className="text-sm text-ink-muted">{t("terminal.settingsLoading")}</p>
            : !save.saved
              ? undefined
              : <p role="status" className="text-sm text-live">{t("terminal.settingsSaved")}</p>)
        : <Notice tone="danger">{save.error}</Notice>}
      >
        <Button kind="primary" disabled={busy} onClick={() => void submit()}>
          {t("terminal.startSave")}
        </Button>
      </ActionArea>
      </fieldset>
    </SettingsSection>
  );
}

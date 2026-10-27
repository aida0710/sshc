import type { LocalShellProfile } from "../api/settings";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import {
  defaultBrowserScrollbackLines,
  maxBrowserScrollbackLines,
  minBrowserScrollbackLines,
} from "../terminal/browserScrollback";
import { fonts } from "../terminal/fonts";
import { palettes } from "../terminal/palettes";
import type { DescribableControlProps } from "../ui/describeControl";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField, control, sizedControl } from "../ui/form";
import { fontSizeRange, type TerminalDraft } from "./terminalSettingsDraft";

// The rows of Settings > Terminal, in the groups the page shows together: on a computer each group (or two)
// is a tab, on a phone each is a card. All of them edit the one draft the page saves.

type GroupProps = {
  draft: TerminalDraft;
  edit: (patch: Partial<TerminalDraft>) => void;
};

const unitText = "text-[12.5px] text-ink-muted";

// The colour scheme, font, size and background image.
export function TerminalLookFields({ draft, edit }: GroupProps) {
  const t = useTranslate();
  return (
    <>
      <FieldRow label={t("terminal.paletteLabel")} hint={t("terminal.paletteHint")}>
        <AppearancePicker
          choices={palettes}
          value={draft.palette}
          onChange={(next) => edit({ palette: next })}
          unchosen={t("terminal.paletteFollowsTheme")}
        />
      </FieldRow>
      <FieldRow label={t("terminal.fontLabel")} hint={t("terminal.fontHint")}>
        <AppearancePicker
          choices={fonts}
          value={draft.font}
          onChange={(next) => edit({ font: next })}
          unchosen={t("terminal.fontFollowsSystem")}
        />
      </FieldRow>
      <FieldRow label={t("terminal.fontSizeLabel")} hint={t("terminal.fontSizeHint")}>
        <FontSizeInput value={draft.fontSize} onChange={(next) => edit({ fontSize: next })} />
      </FieldRow>
      <FieldRow label={t("terminal.backgroundLabel")} hint={t("terminal.backgroundHint")} interactiveChildren>
        <BackgroundPicker
          value={draft.background}
          onChange={(next) => edit({ background: next })}
          tint={draft.tint}
          onTintChange={(next) => edit({ tint: next })}
          unchosen={t("terminal.backgroundNone")}
        />
      </FieldRow>
    </>
  );
}

// FontSizeInput is the size and its unit. FieldRow describes the control it is given, so the description is
// passed on to the input. The input keeps its own name so the unit is not read as part of it.
function FontSizeInput({ value, onChange, ...description }: { value: string; onChange: (value: string) => void } & DescribableControlProps) {
  const t = useTranslate();
  return (
    <span className="flex items-center gap-2">
      <input
        type="number"
        min={fontSizeRange.min}
        max={fontSizeRange.max}
        aria-label={t("terminal.fontSizeLabel")}
        {...description}
        className={`${sizedControl("w-[90px]")} font-mono`}
        value={value}
        placeholder="15 / 13"
        onChange={(event) => onChange(event.target.value)}
      />
      <span aria-hidden="true" className={unitText}>px</span>
    </span>
  );
}

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

// Reconnecting, the connection log, and the three limits on one row.
export function TerminalConnectionFields({ draft, edit }: GroupProps) {
  const t = useTranslate();
  return (
    <>
      <FieldRow label={t("terminal.reconnectLabel")} hint={t("terminal.reconnectHint")}>
        <select className={control} value={draft.reconnect} onChange={(event) => edit({ reconnect: event.target.value })}>
          {reconnectChoices.map((choice) => <option key={choice.value} value={choice.value}>{t(choice.label)}</option>)}
        </select>
      </FieldRow>
      <FieldRow label={t("terminal.verbosityLabel")} hint={t("terminal.verbosityHint")}>
        <select className={control} value={draft.verbosity} onChange={(event) => edit({ verbosity: event.target.value })}>
          {verbosityChoices.map((choice) => <option key={choice.value} value={choice.value}>{t(choice.label)}</option>)}
        </select>
      </FieldRow>
      <FieldRow
        label={t("terminal.limitsLabel")}
        hint={t("terminal.limitsHint", {
          scrollbackMin: minBrowserScrollbackLines,
          scrollbackMax: maxBrowserScrollbackLines,
          scrollbackDefault: defaultBrowserScrollbackLines,
        })}
        interactiveChildren
      >
        <TerminalLimitInputs draft={draft} edit={edit} />
      </FieldRow>
    </>
  );
}

// TerminalLimitInputs are the three numbers in one sentence-like line. Each input keeps its full name (the
// short words beside them only make sense together), and all three take the row's description.
function TerminalLimitInputs({ draft, edit, ...description }: GroupProps & DescribableControlProps) {
  const t = useTranslate();
  return (
    <span className={`flex flex-wrap items-center gap-x-[18px] gap-y-2 ${unitText}`}>
      <label className="inline-flex items-center gap-2">
        {t("terminal.maxSessionsShort")}
        <input
          type="number"
          min={1}
          max={200}
          aria-label={t("terminal.maxSessionsLabel")}
          {...description}
          className={`${sizedControl("w-[72px]")} font-mono`}
          value={draft.maxSessions}
          placeholder="50"
          onChange={(event) => edit({ maxSessions: event.target.value })}
        />
      </label>
      <label className="inline-flex items-center gap-2">
        {t("terminal.scrollbackShort")}
        <input
          type="number"
          min={16384}
          max={4194304}
          aria-label={t("terminal.scrollbackLabel")}
          {...description}
          className={`${sizedControl("w-[110px]")} font-mono`}
          value={draft.scrollback}
          placeholder="262144"
          onChange={(event) => edit({ scrollback: event.target.value })}
        />
        {t("terminal.bytesUnit")}
      </label>
      <label className="inline-flex items-center gap-2">
        {t("terminal.browserScrollbackShort")}
        <input
          type="number"
          min={minBrowserScrollbackLines}
          max={maxBrowserScrollbackLines}
          aria-label={t("terminal.browserScrollbackLabel")}
          {...description}
          className={`${sizedControl("w-[84px]")} font-mono`}
          value={draft.browserScrollbackLines}
          placeholder={String(defaultBrowserScrollbackLines)}
          onChange={(event) => edit({ browserScrollbackLines: event.target.value })}
        />
        {t("terminal.linesUnit")}
      </label>
    </span>
  );
}

// Where a local shell starts and which shell it is.
export function TerminalStartFields({ draft, edit, profiles }: GroupProps & { profiles: LocalShellProfile[] }) {
  const t = useTranslate();
  return (
    <>
      <FieldRow label={t("terminal.startLabel")} hint={t("terminal.startHint")}>
        <input
          type="text"
          className={`${control} font-mono`}
          value={draft.startDirectory}
          spellCheck={false}
          placeholder="~/"
          onChange={(event) => edit({ startDirectory: event.target.value })}
        />
      </FieldRow>
      <FieldRow label={t("terminal.localShellProfileLabel")}>
        <select
          className={control}
          value={draft.localShellProfile}
          disabled={profiles.length === 0}
          onChange={(event) => edit({ localShellProfile: event.target.value })}
        >
          <option value="">{t("terminal.localShellProfileSystem")}</option>
          {profiles.filter((profile) => profile.id !== "default").map((profile) => (
            <option key={profile.id} value={profile.id}>{profile.label} — {profile.path}</option>
          ))}
        </select>
      </FieldRow>
    </>
  );
}

type Switch = "copyOnSelect" | "rightClickPaste" | "osc52" | "jisYenBackslash" | "composerEnterSends" | "webgl";

// In reading order: two to a row on a computer.
const switches: { key: Switch; label: MessageKey; hint: MessageKey }[] = [
  { key: "copyOnSelect", label: "terminal.copyOnSelectLabel", hint: "terminal.copyOnSelectHint" },
  { key: "rightClickPaste", label: "terminal.rightClickPasteLabel", hint: "terminal.rightClickPasteHint" },
  { key: "osc52", label: "terminal.osc52DefaultLabel", hint: "terminal.osc52DefaultHint" },
  { key: "jisYenBackslash", label: "terminal.jisYenBackslashLabel", hint: "terminal.jisYenBackslashHint" },
  { key: "composerEnterSends", label: "terminal.composerEnterSendsLabel", hint: "terminal.composerEnterSendsHint" },
  { key: "webgl", label: "terminal.webglLabel", hint: "terminal.webglHint" },
];

// The clipboard, keyboard and drawing switches, in two columns with a line between rows. The first row of
// each column has no line above it, as the first row of a form.
export function TerminalSwitches({ draft, edit }: GroupProps) {
  const t = useTranslate();
  return (
    <div className="grid lg:grid-cols-2 lg:gap-x-6">
      {switches.map(({ key, label, hint }) => (
        <div key={key} className="border-t border-line py-3 first:border-t-0 first:pt-0.5 lg:[&:nth-child(2)]:border-t-0 lg:[&:nth-child(2)]:pt-0.5">
          <CheckboxField label={t(label)} hint={t(hint)} checked={draft[key]} onChange={(checked) => edit({ [key]: checked })} />
        </div>
      ))}
    </div>
  );
}

import type { CSSProperties } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { chosenOption, customAccent, presetAccents, type AccentOption } from "../theme/accent";
import { parseAccentColor } from "../theme/accentColor";
import { accentChoiceOf, withAccent } from "../theme/accentContext";
import { useTheme } from "../theme/context";
import { defaultTheme, isFirstDarkTheme, isTheme, themeAttributes, themeLabel, themes, type Theme } from "../theme/theme";
import { ColorCodeInput } from "./ColorCodeInput";
import { RemoteSessionColorField } from "./RemoteSessionColorField";
import { SettingsSection } from "./SettingsSection";
import { FieldRow } from "../ui/FieldRow";
import { useSettingsDraft } from "./settingsDraftContext";
import { standardPresetId, type SettingsPreset } from "./settingsStore";

const accentLabelKeys: Record<AccentOption, MessageKey> = {
  lavender: "appearance.accent.lavender",
  blue: "appearance.accent.blue",
  sky: "appearance.accent.sky",
  pink: "appearance.accent.pink",
  mono: "appearance.accent.mono",
  custom: "appearance.accent.custom",
};

const accentOptions: readonly AccentOption[] = [...presetAccents, customAccent];

// A custom accent chosen for the first time starts from the colour on screen, so that choosing it changes
// nothing until the colour is moved. The preset values are #rrggbb in index.css; without the stylesheet
// (in tests) it starts from the light lavender.
function accentColorOnScreen(): string {
  return parseAccentColor(getComputedStyle(document.documentElement).getPropertyValue("--ui-accent")) ?? "#6550d0"; // palette-exempt
}

// The look of the app in the preset being edited: the theme, the accent colour, the remote session frame colour,
// the art behind Home and whether to reduce motion. The screen changes once the choices are saved.
export function AppearanceSettingsSection({ presetId, showHeading }: { presetId: string; showHeading: boolean }) {
  const t = useTranslate();
  const { document, editPreset } = useSettingsDraft();
  const preset: SettingsPreset = document?.presets.find((candidate) => candidate.id === presetId) ?? { id: presetId };
  const themeOnScreen = useTheme().theme;
  const storedTheme = preset.appearance?.theme;
  // While the preset this machine uses has chosen no theme, the one on screen is in force.
  const inForce = presetId === (document?.presetInUse ?? standardPresetId) ? themeOnScreen : defaultTheme;
  const theme: Theme = isTheme(storedTheme) ? storedTheme : inForce;
  const accent = accentChoiceOf(preset);
  const chosen = chosenOption(accent);

  function setTheme(next: Theme) {
    editPreset(presetId, "appearance.theme", (current) => ({ ...current, appearance: { ...current.appearance, theme: next } }));
  }

  function choose(next: AccentOption) {
    if (next === customAccent) chooseColor(accentColorOnScreen());
    else editPreset(presetId, "appearance.accent", (current) => withAccent(current, { accent: next, customColor: null }));
  }

  function chooseColor(color: string) {
    editPreset(presetId, "appearance.accent", (current) => withAccent(current, { accent: accentChoiceOf(current).accent, customColor: color }));
  }

  return (
    <SettingsSection id="settings-appearance" label={t("appearance.heading")} icon="appearance" showHeading={showHeading}>
      <FieldRow label={t("appearance.theme")} interactiveChildren>
        <div className="grid max-w-[720px] grid-cols-2 gap-3.5 md:grid-cols-[repeat(auto-fill,minmax(6.5rem,1fr))]" role="radiogroup" aria-label={t("appearance.theme")}>
          {themes.map((option) => (
            <label key={option} className={`flex min-w-0 cursor-pointer flex-col gap-2 text-ui ${isFirstDarkTheme(option) ? "col-start-1" : ""}`}>
              <input
                type="radio"
                name="appearance-theme"
                value={option}
                checked={theme === option}
                onChange={() => setTheme(option)}
                className="peer sr-only"
              />
              <ThemeThumbnail theme={option} />
              <span className="flex items-center gap-2 text-ink-muted peer-checked:text-ink peer-checked:[&>span:first-child]:border-[5px] peer-checked:[&>span:first-child]:border-accent">
                <span aria-hidden="true" className="size-4 shrink-0 rounded-full border-[1.5px] border-ink-faint" />
                {/* A name too long for its column ("システムに合わせる") breaks into even halves, not after its last word. */}
                <span className="min-w-0 text-balance">{t(themeLabel(option))}</span>
              </span>
            </label>
          ))}
        </div>
      </FieldRow>
      <FieldRow label={t("appearance.accent")} hint={t("appearance.accentHint")} interactiveChildren>
        <div>
          <div className="flex flex-wrap gap-[18px]" role="radiogroup" aria-label={t("appearance.accent")}>
            {accentOptions.map((option) => (
              <label key={option} className="flex w-16 cursor-pointer flex-col items-center gap-2 text-xs text-ink-muted has-[:checked]:text-ink">
                <input
                  type="radio"
                  name="appearance-accent"
                  value={option}
                  checked={chosen === option}
                  onChange={() => choose(option)}
                  className="peer sr-only"
                />
                {/* A preset swatch takes its colour from index.css through data-accent, for the theme on screen.
                    The custom swatch is its colour, or every hue until one is chosen. */}
                <span
                  aria-hidden="true"
                  data-accent={option === customAccent && accent.customColor === null ? undefined : option}
                  className={`size-9 rounded-full ring-offset-2 ring-offset-sidebar peer-checked:ring-2 peer-checked:ring-ink peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-accent ${
                    option === customAccent && accent.customColor === null ? "sshc-accent-swatch-unset" : "bg-accent"
                  }`}
                />
                {t(accentLabelKeys[option])}
              </label>
            ))}
          </div>
          {accent.customColor !== null ? (
            <ColorCodeInput
              color={accent.customColor}
              labels={{ picker: t("appearance.customColor"), code: t("appearance.customColorCode"), lowContrast: t("appearance.customColorLowContrast") }}
              // 設定の板はサイドバーの色。
              contrastAgainst="--ui-sidebar"
              onChange={chooseColor}
            />
          ) : null}
        </div>
      </FieldRow>
      <RemoteSessionColorField presetId={presetId} />
    </SettingsSection>
  );
}

// ThemeThumbnail is a small picture of the app in a theme: a sidebar with the
// current item in the accent, and a terminal board. "System" shows both halves.
// Each half carries the theme's attributes so index.css paints it in that theme
// whatever the page is in. The accent stripe reads the page's accent, copied into
// --thumbnail-accent outside the halves, because a half's theme block would
// otherwise put back the default accent.
function ThemeThumbnail({ theme }: { theme: Theme }) {
  const halves = theme === "system" ? (["light", "dark"] as const) : [theme];
  return (
    <span
      aria-hidden="true"
      style={{ "--thumbnail-accent": "var(--ui-accent)" } as CSSProperties}
      className="flex h-24 overflow-hidden rounded-inset border border-line peer-checked:outline-2 peer-checked:outline-offset-2 peer-checked:outline-accent peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent"
    >
      {halves.map((half) => (
        <span key={half} {...themeAttributes(half)} className="flex flex-1 bg-canvas">
          <span className="flex w-[30%] flex-col gap-1.5 bg-sidebar px-1.5 py-2">
            <span className="block h-1.5 w-[70%] rounded-full bg-[var(--thumbnail-accent)]" />
            <span className="block h-1.5 rounded-full bg-hover" />
            <span className="block h-1.5 w-[80%] rounded-full bg-hover" />
          </span>
          <span className="m-1.5 flex-1 rounded-sm border border-line bg-term-bg" />
        </span>
      ))}
    </span>
  );
}

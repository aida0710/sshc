import type { CSSProperties } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { chosenOption, customAccent, presetAccents, type AccentOption } from "../theme/accent";
import { parseAccentColor } from "../theme/accentColor";
import { useAccent } from "../theme/accentContext";
import { useTheme } from "../theme/context";
import { themes, type Theme } from "../theme/theme";
import { Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { CustomAccentColor } from "./CustomAccentColor";
import { SettingsSection } from "./SettingsSection";
import { formColumns } from "../ui/FieldRow";

const themeLabelKeys: Record<Theme, MessageKey> = {
  system: "shell.themeSystem",
  light: "shell.themeLight",
  dark: "shell.themeDark",
};

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

// The look of the app: the theme, kept in this browser like the Menu's switch,
// and the accent colour, kept in sshc's metadata so every machine shows it.
export function AppearanceSettingsSection({ showHeading }: { showHeading: boolean }) {
  const t = useTranslate();
  const { theme, setTheme } = useTheme();
  const { accent, customColor, chooseAccent, previewCustomColor, chooseCustomColor } = useAccent();
  const chosen = chosenOption({ accent, customColor });
  const saving = useAsyncOperation();
  const describeFailure = { describe: () => t("appearance.accentSaveFailed") };

  function choose(next: AccentOption) {
    if (next === chosen) return;
    if (next === customAccent) {
      void saving.run(() => chooseCustomColor(accentColorOnScreen()), describeFailure);
      return;
    }
    void saving.run(() => chooseAccent(next), describeFailure);
  }

  function chooseColor(color: string) {
    void saving.run(() => chooseCustomColor(color), describeFailure);
  }

  return (
    <SettingsSection id="settings-appearance" label={t("appearance.heading")} icon="appearance" showHeading={showHeading}>
      {/* Each choice is a row: its name on the left, the options on the right, a line between rows. */}
      <div className={fieldRow}>
        <div className={fieldLabel}>{t("appearance.theme")}</div>
        <div className="grid max-w-[532px] grid-cols-3 gap-3.5" role="radiogroup" aria-label={t("appearance.theme")}>
          {themes.map((option) => (
            <label key={option} className="flex min-w-0 cursor-pointer flex-col gap-2 text-[13px]">
              <input
                type="radio"
                name="appearance-theme"
                value={option}
                checked={theme === option}
                onChange={() => setTheme(option)}
                className="peer sr-only"
              />
              <ThemeThumbnail theme={option} />
              <span className="flex items-center gap-2 text-ink-muted peer-checked:text-ink peer-checked:[&>span]:border-[5px] peer-checked:[&>span]:border-accent">
                <span aria-hidden="true" className="size-4 shrink-0 rounded-full border-[1.5px] border-ink-faint" />
                {t(themeLabelKeys[option])}
              </span>
            </label>
          ))}
        </div>
      </div>
      <div className={fieldRow}>
        <div className={fieldLabel}>
          {t("appearance.accent")}
          <p className="mt-1 text-xs font-normal leading-5 text-ink-faint">{t("appearance.accentHint")}</p>
        </div>
        <div>
          <div className="flex flex-wrap gap-[18px]" role="radiogroup" aria-label={t("appearance.accent")}>
            {accentOptions.map((option) => (
              <label key={option} className="flex w-16 cursor-pointer flex-col items-center gap-2 text-[12.5px] text-ink-muted has-[:checked]:text-ink">
                <input
                  type="radio"
                  name="appearance-accent"
                  value={option}
                  checked={chosen === option}
                  disabled={saving.busy}
                  onChange={() => choose(option)}
                  className="peer sr-only"
                />
                {/* A preset swatch takes its colour from index.css through data-accent, for the theme on screen.
                    The custom swatch is its colour, or every hue until one is chosen. */}
                <span
                  aria-hidden="true"
                  data-accent={option === customAccent && customColor === null ? undefined : option}
                  className={`size-9 rounded-full ring-offset-2 ring-offset-sidebar peer-checked:ring-2 peer-checked:ring-ink peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-accent ${
                    option === customAccent && customColor === null ? "sshc-accent-swatch-unset" : "bg-accent"
                  }`}
                />
                {t(accentLabelKeys[option])}
              </label>
            ))}
          </div>
          {customColor !== null ? (
            <CustomAccentColor color={customColor} disabled={saving.busy} onPreview={previewCustomColor} onChoose={chooseColor} />
          ) : null}
          {saving.error === "" ? null : <div className="mt-4"><Notice tone="danger">{saving.error}</Notice></div>}
        </div>
      </div>
    </SettingsSection>
  );
}

const fieldRow = `grid gap-4 border-t border-line py-5 lg:gap-6 ${formColumns}`;
const fieldLabel = "pt-1.5 text-[13.5px] font-medium text-ink";

// ThemeThumbnail is a small picture of the app in a theme: a sidebar with the
// current item in the accent, and a terminal board. "System" shows both halves.
// Each half carries data-theme so index.css paints it in that theme whatever the
// page is in. The accent stripe reads the page's accent, copied into
// --thumbnail-accent outside the halves, because a half's theme block would
// otherwise put back the default accent.
function ThemeThumbnail({ theme }: { theme: Theme }) {
  const halves = theme === "system" ? (["light", "dark"] as const) : ([theme] as const);
  return (
    <span
      aria-hidden="true"
      style={{ "--thumbnail-accent": "var(--ui-accent)" } as CSSProperties}
      className="flex h-24 overflow-hidden rounded-[10px] border border-line peer-checked:outline-2 peer-checked:outline-offset-2 peer-checked:outline-accent peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent"
    >
      {halves.map((half) => (
        <span key={half} data-theme={half} className="flex flex-1 bg-canvas">
          <span className="flex w-[30%] flex-col gap-1.5 bg-sidebar px-1.5 py-2">
            <span className="block h-1.5 w-[70%] rounded-full bg-[var(--thumbnail-accent)]" />
            <span className="block h-1.5 rounded-full bg-hover" />
            <span className="block h-1.5 w-[80%] rounded-full bg-hover" />
          </span>
          <span className="m-1.5 flex-1 rounded border border-line bg-term-bg" />
        </span>
      ))}
    </span>
  );
}

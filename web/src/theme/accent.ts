import type { components } from "../api/schema";
import { readableInkOn } from "./accentColor";

// Accent is the name of a preset colour for the current item, buttons and the
// focused pane. The preset colours live in index.css under [data-accent]; this
// file only knows their names, which come from the engine's metadata. A colour
// of the user's own is kept beside the name (AccentChoice.customColor), which an
// older sshc that knows only the names reads past and shows the preset instead.
export type Accent = NonNullable<components["schemas"]["AppearanceSettings"]["accent"]>;

// The order the settings page shows the presets in. Amber, green and red are
// missing on purpose: they mean an unread notification, connected and an error.
export const presetAccents = ["lavender", "blue", "sky", "pink", "mono"] as const satisfies readonly Accent[];

export const defaultAccent: Accent = "lavender";

export function isAccent(value: unknown): value is Accent {
  return typeof value === "string" && (presetAccents as readonly string[]).includes(value);
}

// customAccent is the settings page's choice of a colour of one's own, and what index.css paints it under.
export const customAccent = "custom";

// AccentOption is one of the choices on the settings page: a preset, or a colour of one's own.
export type AccentOption = Accent | typeof customAccent;

// AccentChoice is what is on screen: a colour of the user's own when customColor is set, the preset
// otherwise. The preset stays chosen underneath, for an older sshc sharing the same metadata.
export type AccentChoice = { accent: Accent; customColor: string | null };

export const defaultAccentChoice: AccentChoice = { accent: defaultAccent, customColor: null };

export function chosenOption(choice: AccentChoice): AccentOption {
  return choice.customColor === null ? choice.accent : customAccent;
}

// applyAccent puts the choice on <html>: the preset's name for index.css, or the custom colour with an ink
// that reads on it as the variables [data-accent="custom"] paints with.
export function applyAccent(root: HTMLElement, choice: AccentChoice): void {
  root.setAttribute("data-accent", chosenOption(choice));
  if (choice.customColor === null) {
    root.style.removeProperty("--ui-custom-accent");
    root.style.removeProperty("--ui-custom-accent-ink");
    return;
  }
  root.style.setProperty("--ui-custom-accent", choice.customColor);
  root.style.setProperty("--ui-custom-accent-ink", readableInkOn(choice.customColor));
}

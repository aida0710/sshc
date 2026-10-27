import type { components } from "../api/schema";
import { readableInkOn } from "./accentColor";

// Accent is the colour the user picked for the current item, buttons and the
// focused pane. The preset colours live in index.css under [data-accent]; this
// file only knows their names, which come from the engine's metadata. The
// custom accent is one colour of the user's own, kept beside the name.
export type Accent = NonNullable<components["schemas"]["AppearanceSettings"]["accent"]>;

// The order the settings page shows the presets in. Amber, green and red are
// missing on purpose: they mean an unread notification, connected and an error.
const presetAccents = ["lavender", "blue", "sky", "pink", "mono"] as const satisfies readonly Accent[];

export const customAccent: Extract<Accent, "custom"> = "custom";

// accentOptions is every accent in the order the settings page shows them: the presets, then the custom one.
export const accentOptions: readonly Accent[] = [...presetAccents, customAccent];

export const defaultAccent: Accent = "lavender";

export function isAccent(value: unknown): value is Accent {
  return typeof value === "string" && (accentOptions as readonly string[]).includes(value);
}

// AccentChoice is what is on screen: the accent, and the custom colour, which stays while a preset is
// chosen so that choosing the custom accent again brings the same colour back. Null is no colour yet.
export type AccentChoice = { accent: Accent; customColor: string | null };

export const defaultAccentChoice: AccentChoice = { accent: defaultAccent, customColor: null };

// applyAccent puts the choice on <html>: the name for index.css, and the custom colour with an ink that
// reads on it as the variables [data-accent="custom"] paints with.
export function applyAccent(root: HTMLElement, choice: AccentChoice): void {
  root.setAttribute("data-accent", choice.accent);
  if (choice.customColor === null) {
    root.style.removeProperty("--ui-custom-accent");
    root.style.removeProperty("--ui-custom-accent-ink");
    return;
  }
  root.style.setProperty("--ui-custom-accent", choice.customColor);
  root.style.setProperty("--ui-custom-accent-ink", readableInkOn(choice.customColor));
}

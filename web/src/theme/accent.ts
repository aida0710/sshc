import type { components } from "../api/schema";

// Accent is the colour the user picked for the current item, buttons and the
// focused pane. The colour values live in index.css under [data-accent]; this
// file only knows the names, which come from the engine's metadata.
export type Accent = NonNullable<components["schemas"]["AppearanceSettings"]["accent"]>;

// The order the settings page shows them in. Amber, green and red are missing
// on purpose: they mean an unread notification, connected and an error.
export const accents = ["lavender", "blue", "sky", "pink", "mono"] as const satisfies readonly Accent[];

export const defaultAccent: Accent = "lavender";

export function isAccent(value: unknown): value is Accent {
  return typeof value === "string" && (accents as readonly string[]).includes(value);
}

export function applyAccent(root: HTMLElement, accent: Accent): void {
  root.setAttribute("data-accent", accent);
}

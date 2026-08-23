import type { components } from "../api/schema";

export type TerminalAppearance = components["schemas"]["TerminalAppearance"];
type HostMetadata = components["schemas"]["HostMetadata"];
export type Resolved = {
  readonly palette: string;
  readonly font: string;
  readonly background: string;
  readonly tint: number | undefined;
};
export const defaultTint = 55;
export function resolveAppearance(
  forConnection: TerminalAppearance | undefined,
  overall: TerminalAppearance | undefined,
): Resolved {
  const pick = (key: "palette" | "font" | "background"): string =>
    forConnection?.[key] !== undefined && forConnection[key] !== ""
      ? forConnection[key]
      : (overall?.[key] ?? "");
  const tint = forConnection?.backgroundTint ?? overall?.backgroundTint;
  return { palette: pick("palette"), font: pick("font"), background: pick("background"), tint };
}
export type AppearanceChange = { [Key in keyof TerminalAppearance]?: TerminalAppearance[Key] | undefined };

export function chooseAppearance(metadata: HostMetadata, change: AppearanceChange): HostMetadata {
  const merged: AppearanceChange = { ...metadata.appearance, ...change };
  const kept = Object.fromEntries(
    Object.entries(merged).filter(([, value]) => value !== undefined && value !== ""),
  ) as TerminalAppearance;
  const { appearance: _dropped, ...rest } = metadata;
  return Object.keys(kept).length === 0 ? rest : { ...rest, appearance: kept };
}
export function appearanceOf(chosen: {
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
}): { appearance?: TerminalAppearance } {
  const appearance: TerminalAppearance = {
    ...(chosen.palette === "" ? {} : { palette: chosen.palette }),
    ...(chosen.font === "" ? {} : { font: chosen.font }),
    ...(chosen.background === "" ? {} : { background: chosen.background }),
    ...(chosen.background === "" || chosen.tint === undefined ? {} : { backgroundTint: chosen.tint }),
  };
  return Object.keys(appearance).length === 0 ? {} : { appearance };
}

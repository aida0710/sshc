import type { TerminalAppearance } from "../api/settings";

// HostSettings is what each connection of an engine asks its terminals for: an appearance and a VPN route, by alias.
export type HostSettings = {
  hostAppearance: Map<string, TerminalAppearance>;
  hostVPN: Map<string, string>;
};

// hostSettingsToDrawWith is the settings an engine's terminals are drawn with now. `current` is what the screens hold,
// and `read` says whether that was read from this engine's SSH configuration: right after the engine is chosen it is
// still empty. Until it is read, the terminals keep the settings they were drawn with before (`drawnWith`): a
// terminal given another font changes its size, and the program in it is told.
export function hostSettingsToDrawWith({ current, read, drawnWith }: { current: HostSettings; read: boolean; drawnWith: HostSettings | undefined }): HostSettings {
  return read || drawnWith === undefined ? current : drawnWith;
}

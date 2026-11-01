import { updatePresetInterface, usePresetInterface } from "../settings/presetInterface";

// Whether a touch terminal underlines every URL and path a tap opens. A finger cannot point at a link to find
// it, so the lines are drawn unless they are turned off; a mouse terminal underlines only the link under the
// pointer either way (linkUnderlines.ts). The choice is about the device in front of the user, so it is kept in
// a preset (the one this machine uses when presetId is left out). The default is not written.
export function saveTouchLinkUnderlines(shown: boolean, presetId?: string): void {
  void updatePresetInterface((settings) => ({ ...settings, hideTouchLinkUnderlines: !shown }), presetId).catch(() => undefined);
}

export function useTouchLinkUnderlines(presetId?: string): boolean {
  return !(usePresetInterface(presetId).hideTouchLinkUnderlines ?? false);
}

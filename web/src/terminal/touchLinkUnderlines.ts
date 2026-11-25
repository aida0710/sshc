import { usePresetInterface } from "../settings/presetInterface";

// Whether a touch terminal underlines every URL and path a tap opens. A finger cannot point at a link to find
// it, so the lines are drawn unless they are turned off; a mouse terminal underlines only the link under the
// pointer either way (linkUnderlines.ts). The choice is about the device in front of the user, so it is kept in
// the preset this machine uses.
export function useTouchLinkUnderlines(): boolean {
  return !(usePresetInterface().hideTouchLinkUnderlines ?? false);
}

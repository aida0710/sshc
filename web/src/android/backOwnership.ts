import { useEffect } from "react";
import { subscribeToAndroidBackClosers } from "../ui/androidBackClosers";
import { isAndroidBackStopOpen } from "../ui/useAndroidBackStop";
import { isDismissibleLayerOpen } from "../ui/useDismissibleLayer";
import { hasTextSelection } from "../ui/textSelection";

// somethingForBackToClose tells whether Android's Back would close something on the page before going anywhere:
// a layer (a dialog, a menu, the navigation), an open part of a page, or selected text (useShellLayers).
export function somethingForBackToClose(): boolean {
  return isDismissibleLayerOpen() || isAndroidBackStopOpen() || hasTextSelection();
}

// useBackOwnership tells the Android app whether Back is the page's. On the top of Home with nothing open, Back
// leaves the app, and the app lets Android take it (NativeBridge.setBackTakenByPage): Android then plays its own
// animation back to the home screen as the person swipes, as for any app at its first screen. Anywhere else the
// page takes Back, to close what is open or to go back a page.
export function useBackOwnership(onTopOfHome: boolean, target: Window = window) {
  useEffect(() => {
    const bridge = target.sshcAndroid;
    if (bridge === undefined) return;
    let reported: boolean | null = null;
    const report = () => {
      const taken = !onTopOfHome || somethingForBackToClose();
      if (taken === reported) return;
      reported = taken;
      bridge.setBackTakenByPage(taken);
    };
    report();
    return subscribeToAndroidBackClosers(report);
  }, [onTopOfHome, target]);
}

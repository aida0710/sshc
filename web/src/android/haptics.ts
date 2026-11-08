// HapticFeedback is the touch feedback the Android app plays (NativeBridge.performHapticFeedback). The system plays
// it, so it follows the phone's own setting for touch vibration.
//   keyPress: a key of the terminal's key bar or the send button, pressed down, as the phone's keyboard does.
// A long press needs none from the page: the WebView plays one itself when the page takes the long press (opens
// its menu, starts choosing rows).
export type HapticFeedback = "keyPress";

// playHaptic plays the feedback in the Android app. A browser plays nothing.
export function playHaptic(feedback: HapticFeedback, target: Window = window): void {
  target.sshcAndroid?.performHapticFeedback(feedback);
}

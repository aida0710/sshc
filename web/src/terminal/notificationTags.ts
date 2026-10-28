// Tags name the session a notification is about, so that a newer notification
// for the same session replaces the older one, and so that a tapped Android
// notification can still find its session after the page reloaded.
const notificationTagPrefix = "sshc-terminal-";

// A program in the terminal asked for attention (OSC 9/99/777), or a long
// command finished (OSC 133).
export function terminalNotificationTag(sessionId: string): string {
  return `${notificationTagPrefix}${sessionId}`;
}

// The session a terminal notification tag names, or null for any other tag.
export function sessionIdFromNotificationTag(tag: string): string | null {
  if (tag.startsWith(notificationTagPrefix) && tag.length > notificationTagPrefix.length) return tag.slice(notificationTagPrefix.length);
  return null;
}

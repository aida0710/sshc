// Tags name what a notification is about, so that a newer notification for the
// same session replaces the older one, and so that a tapped Android
// notification can still find its session after the page reloaded.
const programNotificationPrefix = "sshc-terminal-";
const longCommandNotificationPrefix = "sshc-command-";

// A program in the terminal asked for attention (OSC 9/99/777).
export function programNotificationTag(sessionId: string): string {
  return `${programNotificationPrefix}${sessionId}`;
}

// A long command finished while sshc was out of sight.
export function longCommandNotificationTag(sessionId: string): string {
  return `${longCommandNotificationPrefix}${sessionId}`;
}

// The session a terminal notification tag names, or null for any other tag.
export function sessionIdFromNotificationTag(tag: string): string | null {
  for (const prefix of [programNotificationPrefix, longCommandNotificationPrefix]) {
    if (tag.startsWith(prefix) && tag.length > prefix.length) return tag.slice(prefix.length);
  }
  return null;
}

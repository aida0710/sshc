// isUserAway tells whether the person is not looking at sshc: its page is
// hidden, or its window is visible but another window has the focus (another
// app beside it). Unread marks, sounds and notifications are for them then.
export function isUserAway(): boolean {
  return document.hidden || !document.hasFocus();
}

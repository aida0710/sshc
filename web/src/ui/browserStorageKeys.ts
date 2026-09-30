// Every key sshc keeps in the browser's storage. Listing them in one place
// shows what a reset has to clear and how the next key should be named.
//
// A new key is named sshc.<area>.<name>.v<version>: the screen or feature
// that owns it, what it holds, and the version of the stored shape, in
// lowercase kebab-case (sshc.connections.list-width.v1). A new shape takes a
// new version and the old key is left unread, so the value starts over from
// its default; nothing is carried over between browser storage keys.
// Keys named before this rule keep their names, because renaming one would
// reset that value in every browser.

// localStorage outlives the tab and is shared by every tab of the origin.
export const localStorageKeys = {
  theme: "sshc.theme",
  locale: "sshc.language",
  browserRegistration: "sshc.browser.registration.v1",
  navigationWidth: "sshc.navigation.width",
  connectionListWidth: "sshc.connections.list-width.v1",
  connectionGroupsWidth: "sshc.connections.groups-width.v1",
  quickConnectView: "sshc.home.quick-connect-view",
  // The SFTP workspace layout is one document: the panes from left to right,
  // each with its tabs and the position of the selected tab. Anything else
  // stored under it restores as a single blank pane.
  sftpPanes: "sshc.sftp.panes.v1",
  sftpSplitRatio: "sshc.sftp.splitRatio",
  sftpQueueView: "sshc.sftp.queueView",
  shortcutBindings: "sshc.shortcuts.v1",
  shortcutPresetSelection: "sshc.shortcuts.selected.v1",
  notificationSound: "sshc.terminal-notification-sound.v1",
} as const;

// sessionStorage belongs to one tab and ends with it.
export const sessionStorageKeys = {
  sessionCSRF: "sshc.session.csrf",
  liveWorkspace: "sshc.terminal.live-workspace.v1",
} as const;

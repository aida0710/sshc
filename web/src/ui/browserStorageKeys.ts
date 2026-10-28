// Every key sshc keeps in the browser's storage. Listing them in one place
// shows what a reset has to clear and how the next key should be named.
//
// A new key is named sshc.<area>.<name>.v<version>: the screen or feature
// that owns it, what it holds, and the version of the stored shape, in
// lowercase kebab-case (sshc.connections.list-width.v2). A new shape takes a
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
  // v2: the width of the connection list alone. v1 also held the groups column, which now sits beside the card.
  connectionListWidth: "sshc.connections.list-width.v2",
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
  // The terminal workspaces folded in the sidebar tree, by engine workspace ID.
  collapsedTerminalWorkspaces: "sshc.terminal.collapsed-workspaces.v1",
  // "true" while hidden terminal workspaces are drawn in the tree as well.
  showHiddenTerminalWorkspaces: "sshc.terminal.show-hidden-workspaces.v1",
  // "true" while this browser keeps the screen on with a terminal shown.
  keepScreenAwake: "sshc.terminal.keep-screen-awake.v1",
  // How terminals take touch in this browser: "auto", "touch" or "mouse".
  touchMode: "sshc.terminal.touch-mode.v1",
  // The SSH alias of the engine whose terminals are shown; empty for this machine.
  runTarget: "sshc.terminal.run-target.v1",
  // The update job whose result dialog this browser has shown, so it opens once.
  updateShownResult: "sshc.update.shown-result.v1",
  // The release whose notice under the version was folded; a newer one unfolds it.
  updateFoldedRelease: "sshc.update.folded-release.v1",
} as const;

// sessionStorage belongs to one tab and ends with it.
export const sessionStorageKeys = {
  sessionCSRF: "sshc.session.csrf",
} as const;

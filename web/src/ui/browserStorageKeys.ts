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
//
// Settings are not kept here: they are synced through the engine (settingsStore.ts).

// localStorage outlives the tab and is shared by every tab of the origin.
export const localStorageKeys = {
  // A copy of the theme and the language in the synced settings, read before the engine answers so the first
  // screen is drawn in them. The synced settings are what counts; these only follow them.
  theme: "sshc.theme",
  locale: "sshc.language",
  browserRegistration: "sshc.browser.registration.v1",
  // The update job whose result dialog this browser has shown, so it opens once.
  updateShownResult: "sshc.update.shown-result.v1",
} as const;

// sessionStorage belongs to one tab and ends with it.
export const sessionStorageKeys = {
  sessionCSRF: "sshc.session.csrf",
} as const;

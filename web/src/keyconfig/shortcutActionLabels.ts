import type { MessageKey } from "../i18n/messages";
import type { ShortcutAction } from "./bindings";

// What each shortcut action is called, on the settings page and in the command palette.
export const shortcutActionLabels: Record<ShortcutAction, MessageKey> = {
  palette: "shortcuts.palette",
  terminalSearch: "shortcuts.terminalSearch",
  copy: "shortcuts.copy",
  paste: "shortcuts.paste",
  previousPrompt: "shortcuts.previousPrompt",
  nextPrompt: "shortcuts.nextPrompt",
  increaseFontSize: "shortcuts.increaseFontSize",
  decreaseFontSize: "shortcuts.decreaseFontSize",
  resetFontSize: "shortcuts.resetFontSize",
  quickCommands: "shortcuts.quickCommands",
  newSession: "shortcuts.newSession",
  closeSession: "shortcuts.closeSession",
  nextSession: "shortcuts.nextSession",
  previousSession: "shortcuts.previousSession",
  nextPane: "shortcuts.nextPane",
  previousPane: "shortcuts.previousPane",
  splitRight: "shortcuts.splitRight",
  splitBelow: "shortcuts.splitBelow",
  toggleNavigation: "shortcuts.toggleNavigation",
  home: "shortcuts.home",
  sftp: "shortcuts.sftp",
};

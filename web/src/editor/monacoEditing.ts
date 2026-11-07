// Load this module only from what loadMonacoEditor.ts loads: it imports Monaco.
import type * as monaco from "monaco-editor/editor/editor.api.js";
import { TabFocus } from "monaco-editor/editor/browser/config/tabFocus.js";

// Holds back the editor features (monacoEditorFeatures.ts) that would change
// the text without the user asking for it:
// - Suggestions show only on Ctrl+Space, not while typing. A suggestion shown
//   while typing takes the Enter or Tab meant for a new line or an indent, and
//   replaces the typed word with another word of the file.
// - A file with unusual line terminators, such as Line Separator (U+2028), is
//   opened as it is. Monaco would otherwise ask in a browser dialog to remove
//   them. The dialog names the file by Monaco's model number and points at a
//   setting sshc does not have, and removing them changes the file.
export const noUnrequestedEditOptions = {
  quickSuggestions: false,
  suggestOnTriggerCharacters: false,
  unusualLineTerminators: "off",
} satisfies monaco.editor.IStandaloneEditorConstructionOptions;

// Monaco's command for the mode in which Tab moves focus instead of
// indenting: Ctrl+M (Ctrl+Shift+M on macOS), or "Toggle Tab Key Moves Focus"
// in the editor's command palette.
const toggleTabMovesFocusCommand = "editor.action.toggleTabFocusMode";

// Monaco keeps the mode in which Tab moves focus for the whole page
// (monacoTabFocus.d.ts), and the editor shows nothing while the mode is on.
// Each editor starts with Tab indenting, so that the mode switched on in one
// file does not keep Tab from indenting in the files opened after it. The mode
// is switched off with Monaco's own command, which also announces to a screen
// reader that Tab now inserts the tab character.
export function startWithTabIndenting(view: monaco.editor.IStandaloneCodeEditor): void {
  if (!TabFocus.getTabFocusMode()) return;
  view.trigger("sshc", toggleTabMovesFocusCommand, null);
}

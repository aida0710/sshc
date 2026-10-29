import type { SerializeAddon } from "@xterm/addon-serialize";
import type { Terminal } from "@xterm/xterm";
import type { Translate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { saveFile, type BrowserSave } from "../ui/saveFile";
import { recentBufferText } from "./buffer";
import { outputText, type CommandBlock } from "./commandBlocks";
import { commandName } from "./commandResult";
import { selectionWithColors } from "./richCopy";
import { scrollbackFileName, scrollbackText } from "./scrollbackFile";

// TerminalCopyActions are the menu's ways to take text out of the terminal, and a command's (TerminalCommandCard).
export type TerminalCopyActions = {
  saveScrollback: () => void;
  copyWithColors: () => void;
  copyContext: () => void;
  copyCommandOutput: (block: CommandBlock) => void;
  saveCommandOutput: (block: CommandBlock) => void;
  // release lets go of the last saved file, once the terminal goes away.
  release: () => void;
};

// terminalCopyActions saves the scrollback to a file, copies the selection with its colors, and copies the
// recent output. notice reports what was done or that there was nothing to take; problem reports a refusal.
export function terminalCopyActions({ view, serializeAddon, title, t, notice, problem }: {
  view: Terminal;
  serializeAddon: SerializeAddon;
  // title names the saved file; it is read when saving, so a renamed session saves under its new name.
  title: () => string;
  t: Translate;
  notice: (message: string) => void;
  problem: (message: string) => void;
}): TerminalCopyActions {
  // The browser may read the last saved file after the click; it is let go
  // once the next save replaces it or the terminal goes away.
  let lastSave: BrowserSave | null = null;
  const saveText = (text: string, name: string) => {
    // The Android app takes a bare media type for its save dialog (NativeBridge.safeMimeType), so no charset.
    void saveFile(new Blob([`${text}\n`], { type: "text/plain" }), scrollbackFileName(name, new Date()))
      .then((save) => {
        lastSave?.release();
        lastSave = save;
      })
      .catch(() => problem(t("terminal.saveScrollbackFailed")));
  };
  return {
    saveScrollback: () => {
      const text = scrollbackText(view.buffer.active);
      if (text === "") {
        notice(t("terminal.saveScrollbackEmpty"));
        return;
      }
      saveText(text, title());
    },
    copyWithColors: () => {
      const copied = selectionWithColors(view, serializeAddon);
      if (copied === null) {
        notice(t("terminal.copyWithColorsEmpty"));
        return;
      }
      void clipboard.writeTextAndHTML(copied.text, copied.html)
        .then(() => notice(t("copy.done")))
        .catch(() => problem(t("terminal.clipboardRefused")));
    },
    copyContext: () => {
      const text = recentBufferText(view.buffer.active);
      if (text === "") {
        notice(t("terminal.copyContextEmpty"));
        return;
      }
      void clipboard.writeText(text)
        .then(() => notice(t("terminal.copyContextDone")))
        .catch(() => problem(t("terminal.clipboardRefused")));
    },
    copyCommandOutput: (block) => {
      const text = outputText(block, view.buffer.active);
      if (text === "") {
        notice(t("terminal.commandOutputEmpty"));
        return;
      }
      void clipboard.writeText(text)
        .then(() => notice(t("copy.done")))
        .catch(() => problem(t("terminal.clipboardRefused")));
    },
    // The file is named after the command, as the scrollback's is after the pane.
    saveCommandOutput: (block) => {
      const text = outputText(block, view.buffer.active);
      if (text === "") {
        notice(t("terminal.commandOutputEmpty"));
        return;
      }
      saveText(text, commandName(block, t));
    },
    release: () => lastSave?.release(),
  };
}

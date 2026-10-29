import type { Dispatch, SetStateAction } from "react";
import type { Terminal } from "@xterm/xterm";
import type { Translate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { trackCommandBlocks, type CommandBlocks } from "./commandBlocks";
import { attachGraphemeClusterMode } from "./graphemeClusterMode";
import { attachKittyClipboard, type KittyClipboard } from "./kittyClipboard";
import { attachOSC7Directory } from "./osc7";
import { attachOsc52Clipboard } from "./osc52";
import { attachOSC9Directory } from "./osc9Directory";
import { attachOSC133Commands } from "./osc133";
import { attachOSC9Progress, noTerminalProgress, type TerminalProgress } from "./osc9Progress";
import { trackPromptCursor, type PromptCursor } from "./promptCursor";
import { trackPromptJumps, type PromptJump } from "./promptJump";
import { attachVisibilityReportQuestions } from "./visibilityReports";

// TerminalSequences are the handlers attached to one terminal, with the ones the view calls into.
export type TerminalSequences = {
  kittyClipboard: KittyClipboard;
  commandBlocks: CommandBlocks;
  prompt: PromptCursor;
  prompts: PromptJump;
  dispose: () => void;
};

// attachTerminalSequences registers what the terminal does with the escape sequences programs send beyond
// drawing: clipboard writes, the working directory, progress, command marks, prompts and the modes the
// engine or this view answer. Handlers registered later see a sequence first (xterm.js), so the order of
// this list matters where two read the same sequence.
export function attachTerminalSequences({ view, t, replaying, clipboardWritesAllowed, reply, notice, problem, setDirectory, setProgress }: {
  view: Terminal;
  t: Translate;
  // replaying is true while history is being redrawn; what it asked for was answered long ago.
  replaying: () => boolean;
  // clipboardWritesAllowed is the OSC 52 setting, which governs the Kitty clipboard protocol too.
  clipboardWritesAllowed: () => boolean;
  // reply sends an answer back to the program, as if typed.
  reply: (sequence: string) => void;
  notice: (message: string) => void;
  problem: (message: string) => void;
  setDirectory: (directory: string) => void;
  setProgress: Dispatch<SetStateAction<TerminalProgress>>;
}): TerminalSequences {
  const detachOsc52 = attachOsc52Clipboard({
    parser: view.parser,
    enabled: () => clipboardWritesAllowed() && !replaying(),
    writeText: (text) => clipboard.writeText(text),
    copied: () => notice(t("terminal.osc52Copied")),
    refused: () => problem(t("terminal.clipboardRefused")),
  });
  const kittyClipboard = attachKittyClipboard({
    parser: view.parser,
    writesAllowed: clipboardWritesAllowed,
    replaying,
    writeClipboard: (items) => clipboard.writeItems(items),
    reply,
    copied: () => notice(t("terminal.osc52Copied")),
  });
  const graphemeClusterMode = attachGraphemeClusterMode(view, { replaying, reply });
  // directory is the working directory as of the output parsed so far, for the commands that start in it;
  // the view's state follows later.
  let directory = "";
  const directoryChanged = (path: string) => {
    directory = path;
    setDirectory(path);
  };
  const osc7Directory = attachOSC7Directory(view.parser, directoryChanged);
  const osc9Directory = attachOSC9Directory(view.parser, directoryChanged);
  const osc9Progress = attachOSC9Progress(view.parser, setProgress);
  // A program that quit without clearing its OSC 9;4 progress leaves no bar
  // behind once the shell reports the command finished. The engine notifies
  // the end of a long command (internal/terminal/command_finished.go).
  const commands = attachOSC133Commands(view.parser, { onCommandCompleted: () => setProgress(noTerminalProgress) });
  const visibilityQuestions = attachVisibilityReportQuestions(view.parser);
  // Registered after the command marks and the working directory, so they see OSC 133 and OSC 7 before
  // those consume them.
  const prompt = trackPromptCursor(view);
  const prompts = trackPromptJumps(view);
  const commandBlocks = trackCommandBlocks(view, { directory: () => directory });
  return {
    kittyClipboard,
    commandBlocks,
    prompt,
    prompts,
    dispose: () => {
      detachOsc52();
      kittyClipboard.dispose();
      graphemeClusterMode.dispose();
      osc7Directory.dispose();
      osc9Directory.dispose();
      osc9Progress.dispose();
      commands.dispose();
      commandBlocks.dispose();
      visibilityQuestions.dispose();
      prompt.dispose();
      prompts.dispose();
    },
  };
}

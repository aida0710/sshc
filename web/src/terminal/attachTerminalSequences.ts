import type { Dispatch, SetStateAction } from "react";
import type { Terminal } from "@xterm/xterm";
import type { Translate } from "../i18n/context";
import { clipboard } from "../ui/clipboard";
import { attachCommandMarkers } from "./commandMarkers";
import { attachGraphemeClusterMode } from "./graphemeClusterMode";
import { attachKittyClipboard, type KittyClipboard } from "./kittyClipboard";
import { attachOSC7Directory } from "./osc7";
import { attachOsc52Clipboard } from "./osc52";
import { attachOSC9Directory } from "./osc9Directory";
import { attachOSC9Progress, noTerminalProgress, type TerminalProgress } from "./osc9Progress";
import { trackPromptCursor, type PromptCursor } from "./promptCursor";
import { trackPromptJumps, type PromptJump } from "./promptJump";
import { attachVisibilityReportQuestions } from "./visibilityReports";

// TerminalSequences are the handlers attached to one terminal, with the ones the view calls into.
export type TerminalSequences = {
  kittyClipboard: KittyClipboard;
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
  const osc7Directory = attachOSC7Directory(view.parser, setDirectory);
  const osc9Directory = attachOSC9Directory(view.parser, setDirectory);
  const osc9Progress = attachOSC9Progress(view.parser, setProgress);
  // A program that quit without clearing its OSC 9;4 progress leaves no bar
  // behind once the shell reports the command finished. The engine notifies
  // the end of a long command (internal/terminal/command_finished.go).
  const commandMarkers = attachCommandMarkers(view, () => setProgress(noTerminalProgress));
  const visibilityQuestions = attachVisibilityReportQuestions(view.parser);
  // Registered after the command marks, so it sees OSC 133 before they consume it.
  const prompt = trackPromptCursor(view);
  const prompts = trackPromptJumps(view);
  return {
    kittyClipboard,
    prompt,
    prompts,
    dispose: () => {
      detachOsc52();
      kittyClipboard.dispose();
      graphemeClusterMode.dispose();
      osc7Directory.dispose();
      osc9Directory.dispose();
      osc9Progress.dispose();
      commandMarkers.dispose();
      visibilityQuestions.dispose();
      prompt.dispose();
      prompts.dispose();
    },
  };
}

import type { ReplayReport } from "./stream";

type WritableTerminal = {
  write(text: string, callback?: () => void): void;
};

// xterm.js reads U+0080–U+009F as C1 controls even when they come as UTF-8
// text: U+009B starts a CSI and U+009D an OSC. xterm and Ghostty ignore them
// there, so output that a program cleaned of ESC cannot still drive the screen.
const utf8C1Controls = /[\u0080-\u009f]/gu;

export function createTerminalOutput(terminal: WritableTerminal) {
  let decoder = new TextDecoder();
  let replayBytesRemaining = 0;
  let replaying = false;
  let disposed = false;

  const writeDecodedText = (decoded: string, isReplay: boolean) => {
    const text = decoded.replace(utf8C1Controls, "");
    if (disposed || text === "") return;
    if (!isReplay) {
      terminal.write(text);
      return;
    }
    // xterm parses asynchronously, so the flag follows the parser rather than
    // the arrival of bytes: it is set only while this history text is parsed,
    // which is when xterm answers the queries the history still holds.
    terminal.write("", () => { replaying = true; });
    terminal.write(text, () => { replaying = false; });
  };

  // continueFrom starts over after output that will not arrive: the engine's screen as it stood where the output
  // goes on, which also starts the parser afresh. An engine older than that sends no screen; output from it can
  // start halfway through CSI or OSC, and CAN abandons the old parser state while keeping the screen as it is.
  const continueFrom = (screen: string | undefined, isReplay: boolean) => {
    decoder = new TextDecoder();
    // The screen is history, like a replay: xterm must not answer what it holds.
    if (screen !== undefined) writeDecodedText(screen, true);
    else writeDecodedText("\u0018", isReplay);
  };

  return {
    beginReplay(replay: ReplayReport) {
      replayBytesRemaining = replay.next - replay.start;
      if (!replay.truncated) return;
      continueFrom(replay.screen, true);
    },
    // skip follows output the engine skipped: like a truncated replay, the
    // output goes on from where the engine's screen stood.
    skip(screen?: string) {
      continueFrom(screen, false);
    },
    // parsed is called once xterm.js has parsed everything written so far.
    write(chunk: Uint8Array, parsed?: () => void) {
      const replayLength = Math.min(replayBytesRemaining, chunk.byteLength);
      replayBytesRemaining -= replayLength;
      if (replayLength > 0) {
        writeDecodedText(decoder.decode(chunk.subarray(0, replayLength), { stream: true }), true);
      }
      if (replayLength < chunk.byteLength) {
        writeDecodedText(decoder.decode(chunk.subarray(replayLength), { stream: true }), false);
      }
      // xterm.js runs write callbacks in order, after the writes queued before them.
      if (parsed !== undefined && !disposed) terminal.write("", parsed);
    },
    isReplaying: () => replaying,
    dispose() {
      disposed = true;
    },
  };
}

import type { ReplayReport } from "./stream";

type WritableTerminal = {
  write(text: string, callback?: () => void): void;
};

export function createTerminalOutput(terminal: WritableTerminal) {
  let decoder = new TextDecoder();
  let replayBytesRemaining = 0;
  let replaying = false;
  let disposed = false;

  const writeDecodedText = (text: string, isReplay: boolean) => {
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

  return {
    beginReplay(replay: ReplayReport) {
      replayBytesRemaining = replay.next - replay.start;
      if (!replay.truncated) return;
      decoder = new TextDecoder();
      // A retained buffer can start halfway through CSI or OSC. CAN abandons
      // the old parser state while keeping the displayed screen intact.
      writeDecodedText("\u0018", true);
    },
    write(chunk: Uint8Array) {
      const replayLength = Math.min(replayBytesRemaining, chunk.byteLength);
      replayBytesRemaining -= replayLength;
      if (replayLength > 0) {
        writeDecodedText(decoder.decode(chunk.subarray(0, replayLength), { stream: true }), true);
      }
      if (replayLength < chunk.byteLength) {
        writeDecodedText(decoder.decode(chunk.subarray(replayLength), { stream: true }), false);
      }
    },
    isReplaying: () => replaying,
    dispose() {
      disposed = true;
    },
  };
}

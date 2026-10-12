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
    // xterm parses asynchronously. Queue this change before its text so live
    // bytes already received cannot enable clipboard writes while history parses.
    terminal.write("", () => { replaying = isReplay; });
    terminal.write(text);
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

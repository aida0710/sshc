import type { ReplayReport, ReplaySize } from "./stream";

type WritableTerminal = {
  write(text: string, callback?: () => void): void;
};

// ReplaySizing lets the history be replayed at the size it was written for. A shell moves its cursor for the width
// it had while a line was edited, so the same bytes at another width overwrite the middle of the line. The output
// asks for each size where the history reached it and says when the history is over, so the terminal takes its
// own size again; xterm.js then rewraps what was written.
export type ReplaySizing = {
  replayAt(size: ReplaySize): void;
  replayed(): void;
};

// xterm.js reads U+0080–U+009F as C1 controls even when they come as UTF-8
// text: U+009B starts a CSI and U+009D an OSC. xterm and Ghostty ignore them
// there, so output that a program cleaned of ESC cannot still drive the screen.
const utf8C1Controls = /[\u0080-\u009f]/gu;

export function createTerminalOutput(terminal: WritableTerminal, sizing?: ReplaySizing) {
  let decoder = new TextDecoder();
  let replayBytesRemaining = 0;
  let replaying = false;
  let disposed = false;

  // The writes go through a queue that can wait. A size must be taken where the history reached it, once
  // xterm.js has parsed what came before; but xterm.js stops parsing what is queued after a write callback that
  // resizes it. So at a size, nothing more is handed to xterm.js until the size has been taken just after that
  // callback (a microtask: xterm.js then has nothing left to parse); the writes that come meanwhile, the
  // history's and live output, wait here in order.
  const waiting: (() => void)[] = [];
  let paused = 0;
  const write = (text: string, callback?: () => void) => {
    if (disposed) return;
    if (paused !== 0) waiting.push(() => terminal.write(text, callback));
    else terminal.write(text, callback);
  };
  const resume = (pause: number) => {
    if (pause !== paused) return;
    paused = 0;
    while (paused === 0 && waiting.length > 0) waiting.shift()!();
  };
  let pauses = 0;
  // pauseFor runs action once xterm.js has parsed everything written before it, and writes nothing after it
  // until then.
  const pauseFor = (action: () => void) => {
    const run = () => {
      const pause = ++pauses;
      paused = pause;
      terminal.write("", () => queueMicrotask(() => {
        // The output goes on even when taking the size fails: a terminal that stops here never draws again.
        try {
          action();
        } finally {
          resume(pause);
        }
      }));
    };
    if (disposed) return;
    if (paused !== 0) waiting.push(run);
    else run();
  };

  // The sizes of the replay being written. replayPosition is where its next byte stands in the session's output;
  // sizesAhead are the sizes it has not reached. sizingRound tells a replay that was abandoned (the stream closed
  // halfway) from the current one: its sizes may still be waiting. holding is true from the start of a sized
  // replay until the terminal is told it is over.
  let replayPosition = 0;
  let sizesAhead: ReplaySize[] = [];
  let sized = false;
  let sizingRound = 0;
  let holding = false;
  const release = () => {
    if (!holding) return;
    holding = false;
    sizing?.replayed();
  };
  const sizeAt = (size: ReplaySize) => {
    const round = sizingRound;
    pauseFor(() => { if (round === sizingRound) sizing?.replayAt(size); });
  };
  const finishSizedReplay = () => {
    if (!sized) return;
    sized = false;
    sizesAhead = [];
    const round = sizingRound;
    pauseFor(() => { if (round === sizingRound) release(); });
  };

  const writeDecodedText = (decoded: string, isReplay: boolean) => {
    const text = decoded.replace(utf8C1Controls, "");
    if (disposed || text === "") return;
    if (!isReplay) {
      write(text);
      return;
    }
    // xterm parses asynchronously, so the flag follows the parser rather than
    // the arrival of bytes: it is set only while this history text is parsed,
    // which is when xterm answers the queries the history still holds.
    write("", () => { replaying = true; });
    write(text, () => { replaying = false; });
  };

  // writeReplayed writes replayed bytes, taking each size where the history reached it.
  const writeReplayed = (bytes: Uint8Array) => {
    let offset = 0;
    while (sized && sizesAhead.length > 0 && sizesAhead[0]!.at < replayPosition + bytes.byteLength) {
      const next = sizesAhead.shift()!;
      const before = Math.max(0, next.at - replayPosition - offset);
      if (before > 0) writeDecodedText(decoder.decode(bytes.subarray(offset, offset + before), { stream: true }), true);
      offset += before;
      sizeAt(next);
    }
    if (offset < bytes.byteLength) writeDecodedText(decoder.decode(bytes.subarray(offset), { stream: true }), true);
    replayPosition += bytes.byteLength;
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
    // beginReplay starts the history. sized replays it at the sizes it was written for: a terminal made afresh
    // has nothing of its own to rewrap, while one that was offline already holds its output at its own size.
    beginReplay(replay: ReplayReport, { sized: sizedReplay = false }: { sized?: boolean } = {}) {
      replayBytesRemaining = replay.next - replay.start;
      replayPosition = replay.start;
      sizingRound += 1;
      sized = sizedReplay && sizing !== undefined && (replay.sizes?.length ?? 0) > 0;
      holding = holding || sized;
      sizesAhead = sized ? [...replay.sizes!] : [];
      // The size in effect where the replay starts is also the one the engine's screen was drawn at. Nothing of
      // the history is written yet, so it is taken at once.
      if (sized && sizesAhead[0]!.at <= replay.start) {
        const first = sizesAhead.shift()!;
        if (!disposed) sizing?.replayAt(first);
      }
      if (replay.truncated) continueFrom(replay.screen, true);
      if (replayBytesRemaining === 0) finishSizedReplay();
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
        writeReplayed(chunk.subarray(0, replayLength));
        if (replayBytesRemaining === 0) finishSizedReplay();
      }
      if (replayLength < chunk.byteLength) {
        writeDecodedText(decoder.decode(chunk.subarray(replayLength), { stream: true }), false);
      }
      // xterm.js runs write callbacks in order, after the writes queued before them.
      if (parsed !== undefined) write("", parsed);
    },
    isReplaying: () => replaying,
    // abandonReplay drops the sizes of a replay whose stream closed before it ended; the terminal takes its own
    // size again at once, and what waited is written.
    abandonReplay() {
      sizingRound += 1;
      sized = false;
      sizesAhead = [];
      release();
      resume(paused);
    },
    dispose() {
      disposed = true;
      waiting.length = 0;
    },
  };
}

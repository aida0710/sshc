
// connectionLost is set when the SSH connection, not the program, ended; the
// code is then -1 and means nothing.
export type ExitReport = { code: number; signal: string; connectionLost?: boolean };
// screen comes with a truncated replay: the sequences that show the screen as it stood at start, written before
// the replayed output (internal/terminal/screen_checkpoint.go). An engine older than this sends none.
export type ReplayReport = { start: number; next: number; end: number; truncated: boolean; screen?: string };

export type StreamHandlers = {
  onReplay: (replay: ReplayReport) => void;
  onOutput: (chunk: Uint8Array) => void;
  // The engine skipped output that left its replay buffer while this browser
  // fell behind; the next output begins at start, and screen shows the screen
  // as it stood there.
  onSkipped: (start: number, screen?: string) => void;
  onExit: (exit: ExitReport) => void;
  // A reconnect replaced the shell behind this stream with a new one.
  onShellReplaced: () => void;
  onClose: () => void;
};

export type TerminalStream = {
  send: (data: string) => void;
  // acknowledge tells the engine that xterm.js has parsed this many more bytes
  // of the output received on this stream. The engine keeps at most a couple of
  // MiB ahead of what was acknowledged (internal/httpserver/output_flow.go).
  acknowledge: (bytes: number) => void;
  // sendBinary sends what xterm.js reports through onBinary, one byte per
  // character: the default-format mouse reports, whose coordinates above 95
  // would turn into two bytes if they were encoded as UTF-8.
  sendBinary: (data: string) => void;
  resize: (cols: number, rows: number) => void;
  // reportShown tells the engine whether this browser shows the terminal, for
  // the programs that asked for visibility reports (mode 2033).
  reportShown: (shown: boolean) => void;
  close: () => void;
};

// Keep browser input messages well below the server's 1 MiB per-message read
// limit. A terminal is a byte stream, so UTF-8 and bracketed-paste sequences
// may safely span WebSocket messages as long as their order is preserved.
const inputFrameBytes = 64 * 1024;
// The parsed output is told in steps of this many bytes: far below the engine's
// 2 MiB window, so the engine never waits for an acknowledgement still to come,
// and rare enough to add almost nothing to interactive traffic.
const acknowledgementStepBytes = 64 * 1024;

// streamPath is where the engine that issued the ticket redeems it: this
// machine's /terminal/stream, or the forwarded stream of a run-target engine.
export function streamURL(ticket: string, streamPath = "/terminal/stream", location: Location = window.location): string {
  const scheme = location.protocol === "https:" ? "wss:" : "ws:";
  return `${scheme}//${location.host}${streamPath}?ticket=${encodeURIComponent(ticket)}`;
}

export function openStream(ticket: string, streamPath: string, handlers: StreamHandlers): TerminalStream {
  const socket = new WebSocket(streamURL(ticket, streamPath));
  socket.binaryType = "arraybuffer";
  const encoder = new TextEncoder();
  let exited = false;
  let parsed = 0;
  let acknowledged = 0;

  let pending: (string | Uint8Array)[] | null = [];
  socket.addEventListener("open", () => {
    const queued = pending ?? [];
    pending = null;
    for (const frame of queued) socket.send(frame);
  });

  const push = (frame: string | Uint8Array) => {
    if (pending !== null) {
      pending.push(frame);
      return;
    }
    if (socket.readyState !== WebSocket.OPEN) return;
    socket.send(frame);
  };

  socket.addEventListener("message", (event: MessageEvent<unknown>) => {
    if (event.data instanceof ArrayBuffer) {
      handlers.onOutput(new Uint8Array(event.data));
      return;
    }
    if (typeof event.data !== "string") return;
    try {
      const message: unknown = JSON.parse(event.data);
      const replay = (message as { replay?: unknown }).replay;
      if (validReplay(replay)) {
        handlers.onReplay(replay);
        return;
      }
      const skipped = (message as { skipped?: { start?: unknown; screen?: unknown } }).skipped;
      if (skipped !== undefined) {
        const { start, screen } = skipped;
        if (typeof start !== "number" || !Number.isSafeInteger(start) || start < 0) return;
        if (typeof screen === "string") handlers.onSkipped(start, screen);
        else handlers.onSkipped(start);
        return;
      }
      if ((message as { shellReplaced?: unknown }).shellReplaced !== undefined) {
        handlers.onShellReplaced();
        return;
      }
      const exit = (message as { exit?: ExitReport }).exit;
      if (exit === undefined) return;
      exited = true;
      handlers.onExit({ code: exit.code, signal: exit.signal, ...(exit.connectionLost === true ? { connectionLost: true } : {}) });
    } catch {
      return;
    }
  });

  const finish = () => {
    if (exited) return;
    exited = true;
    handlers.onClose();
  };
  socket.addEventListener("close", finish);
  socket.addEventListener("error", finish);

  const pushInput = (bytes: Uint8Array) => {
    for (let offset = 0; offset < bytes.byteLength; offset += inputFrameBytes) {
      push(bytes.subarray(offset, offset + inputFrameBytes));
    }
  };

  return {
    send(data) {
      pushInput(encoder.encode(data));
    },
    sendBinary(data) {
      pushInput(Uint8Array.from(data, (character) => character.charCodeAt(0) & 0xff));
    },
    resize(cols, rows) {
      push(JSON.stringify({ resize: { cols, rows } }));
    },
    reportShown(shown) {
      push(JSON.stringify({ visible: shown }));
    },
    acknowledge(bytes) {
      parsed += bytes;
      if (parsed - acknowledged < acknowledgementStepBytes) return;
      acknowledged = parsed;
      push(JSON.stringify({ ack: parsed }));
    },
    close() {
      exited = true;
      pending = null;
      socket.close();
    },
  };
}

function validReplay(value: unknown): value is ReplayReport {
  if (typeof value !== "object" || value === null) return false;
  const replay = value as Partial<ReplayReport>;
  return Number.isSafeInteger(replay.start) && Number.isSafeInteger(replay.next) &&
    Number.isSafeInteger(replay.end) && replay.truncated !== undefined &&
    typeof replay.truncated === "boolean" && replay.start! >= 0 &&
    replay.start! <= replay.next! && replay.next! <= replay.end! &&
    (replay.screen === undefined || typeof replay.screen === "string");
}

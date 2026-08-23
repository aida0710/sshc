
export type ExitReport = { code: number; signal: string };

export type StreamHandlers = {
  onOutput: (chunk: Uint8Array) => void;
  onExit: (exit: ExitReport) => void;
  onClose: () => void;
};

export type TerminalStream = {
  send: (data: string) => void;
  resize: (cols: number, rows: number) => void;
  close: () => void;
};

export function streamURL(ticket: string, location: Location = window.location): string {
  const scheme = location.protocol === "https:" ? "wss:" : "ws:";
  return `${scheme}//${location.host}/terminal/stream?ticket=${encodeURIComponent(ticket)}`;
}

export function openStream(ticket: string, handlers: StreamHandlers): TerminalStream {
  const socket = new WebSocket(streamURL(ticket));
  socket.binaryType = "arraybuffer";
  const encoder = new TextEncoder();
  let exited = false;

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
      const exit = (message as { exit?: ExitReport }).exit;
      if (exit === undefined) return;
      exited = true;
      handlers.onExit({ code: exit.code, signal: exit.signal });
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

  return {
    send(data) {
      push(encoder.encode(data));
    },
    resize(cols, rows) {
      push(JSON.stringify({ resize: { cols, rows } }));
    },
    close() {
      exited = true;
      pending = null;
      socket.close();
    },
  };
}

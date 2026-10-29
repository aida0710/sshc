import { describe, expect, it, vi } from "vitest";
import { openStream, streamURL, type StreamHandlers } from "./stream";

describe("streamURL", () => {
  it("stays on the same origin and carries only the ticket", () => {
    const url = streamURL("one-time", "/terminal/stream", { protocol: "http:", host: "127.0.0.1:51234" } as Location);

    expect(url).toBe("ws://127.0.0.1:51234/terminal/stream?ticket=one-time");
  });

  it("follows the page scheme and escapes the ticket", () => {
    const url = streamURL("a b&c", "/terminal/stream", { protocol: "https:", host: "127.0.0.1:1" } as Location);

    expect(url).toBe("wss://127.0.0.1:1/terminal/stream?ticket=a%20b%26c");
  });

  it("goes to the forwarded stream of a run-target engine", () => {
    const url = streamURL("t", "/engines/r540/terminal/stream", { protocol: "http:", host: "127.0.0.1:54447" } as Location);

    expect(url).toBe("ws://127.0.0.1:54447/engines/r540/terminal/stream?ticket=t");
  });
});

type Listener = (event: unknown) => void;

class FakeSocket {
  static last: FakeSocket | null = null;
  readyState = 0;
  binaryType = "";
  sent: unknown[] = [];
  closed = false;
  private listeners: Record<string, Listener[]> = {};

  constructor(public url: string) {
    FakeSocket.last = this;
  }

  addEventListener(name: string, listener: Listener) {
    (this.listeners[name] ??= []).push(listener);
  }

  send(payload: unknown) {
    this.sent.push(payload);
  }

  close() {
    this.closed = true;
  }

  emit(name: string, event: unknown) {
    for (const listener of this.listeners[name] ?? []) listener(event);
  }

  open() {
    this.readyState = 1;
    this.emit("open", {});
  }
}

function withFakeSocket(): typeof FakeSocket {
  vi.stubGlobal("WebSocket", FakeSocket);
  Object.assign(FakeSocket, { OPEN: 1 });
  return FakeSocket;
}

function streamHandlers(chosen: Partial<StreamHandlers> = {}): StreamHandlers {
  return { onReplay: vi.fn(), onOutput: vi.fn(), onSkipped: vi.fn(), onExit: vi.fn(), onShellReplaced: vi.fn(), onClose: vi.fn(), ...chosen };
}

describe("openStream", () => {
  it("delivers binary frames as output and text frames as control", () => {
    withFakeSocket();
    const onOutput = vi.fn();
    const onReplay = vi.fn();
    const onExit = vi.fn();
    const onClose = vi.fn();
    const onShellReplaced = vi.fn();
    const onSkipped = vi.fn();
    openStream("one-time", "/terminal/stream", streamHandlers({ onReplay, onOutput, onSkipped, onExit, onShellReplaced, onClose }));
    const socket = FakeSocket.last!;

    expect(socket.binaryType).toBe("arraybuffer");

    socket.emit("message", { data: JSON.stringify({ shellReplaced: {} }) });
    expect(onShellReplaced).toHaveBeenCalledOnce();

    socket.emit("message", { data: JSON.stringify({ replay: { start: 10, next: 15, end: 15, truncated: false } }) });
    expect(onReplay).toHaveBeenCalledWith({ start: 10, next: 15, end: 15, truncated: false });

    const screen = "\u001b[H\u001b[2Jdrawn earlier";
    socket.emit("message", { data: JSON.stringify({ replay: { start: 20, next: 25, end: 25, truncated: true, screen } }) });
    expect(onReplay).toHaveBeenLastCalledWith({ start: 20, next: 25, end: 25, truncated: true, screen });
    socket.emit("message", { data: JSON.stringify({ replay: { start: 20, next: 25, end: 25, truncated: true, screen: 7 } }) });
    expect(onReplay).toHaveBeenCalledTimes(2);

    socket.emit("message", { data: JSON.stringify({ skipped: { start: -1 } }) });
    socket.emit("message", { data: JSON.stringify({ skipped: { start: 20 } }) });
    expect(onSkipped).toHaveBeenCalledExactlyOnceWith(20);
    socket.emit("message", { data: JSON.stringify({ skipped: { start: 30, screen } }) });
    expect(onSkipped).toHaveBeenLastCalledWith(30, screen);

    const payload = new TextEncoder().encode("hello");
    const buffer = new ArrayBuffer(payload.byteLength);
    new Uint8Array(buffer).set(payload);
    socket.emit("message", { data: buffer });
    expect(onOutput).toHaveBeenCalledOnce();
    expect(new TextDecoder().decode(onOutput.mock.calls[0]?.[0] as Uint8Array)).toBe("hello");

    socket.emit("message", { data: JSON.stringify({ exit: { code: 255, signal: "" } }) });
    expect(onExit).toHaveBeenCalledWith({ code: 255, signal: "" });
  });

  it("reports that the SSH connection, not the program, ended", () => {
    withFakeSocket();
    const onExit = vi.fn();
    openStream("one-time", "/terminal/stream", streamHandlers({ onExit }));
    const socket = FakeSocket.last!;
    socket.open();

    socket.emit("message", { data: JSON.stringify({ exit: { code: -1, signal: "", connectionLost: true } }) });

    expect(onExit).toHaveBeenCalledWith({ code: -1, signal: "", connectionLost: true });
  });

  it("tells the engine whether this browser shows the terminal", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;
    socket.open();

    stream.reportShown(false);
    stream.reportShown(true);

    expect(socket.sent.map((frame) => JSON.parse(String(frame)))).toEqual([{ visible: false }, { visible: true }]);
  });

  it("sends keystrokes as bytes and resizes as JSON", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;
    socket.open();

    stream.send("echo hi\r");
    stream.resize(120, 34);

    expect(new TextDecoder().decode(socket.sent[0] as Uint8Array)).toBe("echo hi\r");
    expect(JSON.parse(String(socket.sent[1]))).toEqual({ resize: { cols: 120, rows: 34 } });
  });

  it("sends xterm's binary reports one byte per character, without UTF-8", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;
    socket.open();

    // A press of the left button at column 128, row 1 in the default format.
    stream.sendBinary("\u001b[M \u00a0!");

    expect([...(socket.sent[0] as Uint8Array)]).toEqual([0x1b, 0x5b, 0x4d, 0x20, 0xa0, 0x21]);
  });

  it("acknowledges the parsed output in steps rather than for every chunk", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;
    socket.open();

    for (let chunk = 0; chunk < 63; chunk += 1) stream.acknowledge(1024);
    expect(socket.sent).toEqual([]);
    stream.acknowledge(1024);
    for (let chunk = 0; chunk < 64; chunk += 1) stream.acknowledge(1024);

    expect(socket.sent.map((frame) => JSON.parse(String(frame)))).toEqual([{ ack: 64 * 1024 }, { ack: 128 * 1024 }]);
  });

  it("splits a large UTF-8 paste below the server's per-message limit without changing bytes", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;
    socket.open();
    const pasted = `\u001b[200~${"日本語".repeat(100_000)}\u001b[201~`;
    const expected = new TextEncoder().encode(pasted);

    stream.send(pasted);

    const frames = socket.sent as Uint8Array[];
    expect(frames.length).toBeGreaterThan(1);
    expect(frames.every((frame) => frame.byteLength <= 64 * 1024)).toBe(true);
    const joined = new Uint8Array(frames.reduce((size, frame) => size + frame.byteLength, 0));
    let offset = 0;
    for (const frame of frames) {
      joined.set(frame, offset);
      offset += frame.byteLength;
    }
    expect(joined.byteLength).toBe(expected.byteLength);
    expect(new TextDecoder().decode(joined)).toBe(pasted);
  });

  it("holds frames sent before the socket opens and delivers them in order", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;

    stream.resize(120, 34);
    stream.send("x");
    expect(socket.sent).toHaveLength(0);

    socket.open();

    expect(JSON.parse(String(socket.sent[0]))).toEqual({ resize: { cols: 120, rows: 34 } });
    expect(new TextDecoder().decode(socket.sent[1] as Uint8Array)).toBe("x");
  });

  it("does not deliver held frames when it was closed before opening", () => {
    withFakeSocket();
    const stream = openStream("one-time", "/terminal/stream", streamHandlers());
    const socket = FakeSocket.last!;

    stream.resize(120, 34);
    stream.close();
    socket.open();

    expect(socket.sent).toHaveLength(0);
  });

  it("separates a dropped connection from an exit", () => {
    withFakeSocket();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", "/terminal/stream", streamHandlers({ onExit, onClose }));

    FakeSocket.last!.emit("close", {});

    expect(onClose).toHaveBeenCalledOnce();
    expect(onExit).not.toHaveBeenCalled();
  });

  it("does not report a close after an exit was already delivered", () => {
    withFakeSocket();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", "/terminal/stream", streamHandlers({ onExit, onClose }));
    const socket = FakeSocket.last!;

    socket.emit("message", { data: JSON.stringify({ exit: { code: 0, signal: "" } }) });
    socket.emit("close", {});

    expect(onExit).toHaveBeenCalledOnce();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("survives a control frame it cannot read", () => {
    withFakeSocket();
    const onClose = vi.fn();
    openStream("one-time", "/terminal/stream", streamHandlers({ onClose }));

    FakeSocket.last!.emit("message", { data: "{not json" });

    expect(onClose).not.toHaveBeenCalled();
  });
});

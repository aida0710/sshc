import { describe, expect, it, vi } from "vitest";
import { openStream, streamURL } from "./stream";

describe("streamURL", () => {
  it("stays on the same origin and carries only the ticket", () => {
    const url = streamURL("one-time", { protocol: "http:", host: "127.0.0.1:51234" } as Location);

    expect(url).toBe("ws://127.0.0.1:51234/terminal/stream?ticket=one-time");
  });

  it("follows the page scheme and escapes the ticket", () => {
    const url = streamURL("a b&c", { protocol: "https:", host: "127.0.0.1:1" } as Location);

    expect(url).toBe("wss://127.0.0.1:1/terminal/stream?ticket=a%20b%26c");
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
  vi.stubGlobal("WebSocket", FakeSocket as unknown as typeof WebSocket);
  Object.assign(FakeSocket, { OPEN: 1 });
  return FakeSocket;
}

describe("openStream", () => {
  it("delivers binary frames as output and text frames as control", () => {
    withFakeSocket();
    const onOutput = vi.fn();
    const onReplay = vi.fn();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", { onReplay, onOutput, onExit, onClose });
    const socket = FakeSocket.last!;

    expect(socket.binaryType).toBe("arraybuffer");

    socket.emit("message", { data: JSON.stringify({ replay: { start: 10, next: 15, end: 15, truncated: false } }) });
    expect(onReplay).toHaveBeenCalledWith({ start: 10, next: 15, end: 15, truncated: false });

    const payload = new TextEncoder().encode("hello");
    const buffer = new ArrayBuffer(payload.byteLength);
    new Uint8Array(buffer).set(payload);
    socket.emit("message", { data: buffer });
    expect(onOutput).toHaveBeenCalledOnce();
    expect(new TextDecoder().decode(onOutput.mock.calls[0]?.[0] as Uint8Array)).toBe("hello");

    socket.emit("message", { data: JSON.stringify({ exit: { code: 255, signal: "" } }) });
    expect(onExit).toHaveBeenCalledWith({ code: 255, signal: "" });
  });

  it("sends keystrokes as bytes and resizes as JSON", () => {
    withFakeSocket();
    const stream = openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
    const socket = FakeSocket.last!;
    socket.open();

    stream.send("echo hi\r");
    stream.resize(120, 34);

    expect(new TextDecoder().decode(socket.sent[0] as Uint8Array)).toBe("echo hi\r");
    expect(JSON.parse(String(socket.sent[1]))).toEqual({ resize: { cols: 120, rows: 34 } });
  });

  it("splits a large UTF-8 paste below the server's per-message limit without changing bytes", () => {
    withFakeSocket();
    const stream = openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
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
    const stream = openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
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
    const stream = openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
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
    openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit, onClose });

    FakeSocket.last!.emit("close", {});

    expect(onClose).toHaveBeenCalledOnce();
    expect(onExit).not.toHaveBeenCalled();
  });

  it("does not report a close after an exit was already delivered", () => {
    withFakeSocket();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit, onClose });
    const socket = FakeSocket.last!;

    socket.emit("message", { data: JSON.stringify({ exit: { code: 0, signal: "" } }) });
    socket.emit("close", {});

    expect(onExit).toHaveBeenCalledOnce();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("survives a control frame it cannot read", () => {
    withFakeSocket();
    const onClose = vi.fn();
    openStream("one-time", { onReplay: vi.fn(), onOutput: vi.fn(), onExit: vi.fn(), onClose });

    FakeSocket.last!.emit("message", { data: "{not json" });

    expect(onClose).not.toHaveBeenCalled();
  });
});

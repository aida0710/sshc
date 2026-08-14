import { describe, expect, it, vi } from "vitest";
import { openStream, streamURL } from "./stream";

// 通信路は /api/ の外にある。ブラウザが WebSocket のハンドシェイクに
// カスタムヘッダを付けられないので、CSRF ヘッダを要求する面には置けない。
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
  // **繋がる前から始まる。** 本物の WebSocket は new した直後 CONNECTING で
  // あり、そこを OPEN として検査すると、開く前に送られたフレームが落ちること
  // に気づけない。
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
  // バイナリフレームが PTY の生バイト列である。base64 を挟まない。
  it("delivers binary frames as output and text frames as control", () => {
    withFakeSocket();
    const onOutput = vi.fn();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", { onOutput, onExit, onClose });
    const socket = FakeSocket.last!;

    expect(socket.binaryType).toBe("arraybuffer");

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
    const stream = openStream("one-time", { onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
    const socket = FakeSocket.last!;
    socket.open();

    stream.send("echo hi\r");
    stream.resize(120, 34);

    expect(new TextDecoder().decode(socket.sent[0] as Uint8Array)).toBe("echo hi\r");
    expect(JSON.parse(String(socket.sent[1]))).toEqual({ resize: { cols: 120, rows: 34 } });
  });

  // 端末を開いた直後の最初のサイズは、まだ CONNECTING のこの通信路を通る。
  // ここで落とすと PTY は 80×24 のまま残り、窓の大きさが変わるまで直らない。
  it("holds frames sent before the socket opens and delivers them in order", () => {
    withFakeSocket();
    const stream = openStream("one-time", { onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
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
    const stream = openStream("one-time", { onOutput: vi.fn(), onExit: vi.fn(), onClose: vi.fn() });
    const socket = FakeSocket.last!;

    stream.resize(120, 34);
    stream.close();
    socket.open();

    expect(socket.sent).toHaveLength(0);
  });

  // 通信路が切れることと、子プロセスが終わることは別の事実である。前者では
  // 同じセッションへ繋ぎ直せるし、後者では終了の理由が読める。
  it("separates a dropped connection from an exit", () => {
    withFakeSocket();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", { onOutput: vi.fn(), onExit, onClose });

    FakeSocket.last!.emit("close", {});

    expect(onClose).toHaveBeenCalledOnce();
    expect(onExit).not.toHaveBeenCalled();
  });

  it("does not report a close after an exit was already delivered", () => {
    withFakeSocket();
    const onExit = vi.fn();
    const onClose = vi.fn();
    openStream("one-time", { onOutput: vi.fn(), onExit, onClose });
    const socket = FakeSocket.last!;

    socket.emit("message", { data: JSON.stringify({ exit: { code: 0, signal: "" } }) });
    socket.emit("close", {});

    expect(onExit).toHaveBeenCalledOnce();
    expect(onClose).not.toHaveBeenCalled();
  });

  // 読めない制御フレームひとつが、生きているセッションを閉じてよい理由はない。
  it("survives a control frame it cannot read", () => {
    withFakeSocket();
    const onClose = vi.fn();
    openStream("one-time", { onOutput: vi.fn(), onExit: vi.fn(), onClose });

    FakeSocket.last!.emit("message", { data: "{not json" });

    expect(onClose).not.toHaveBeenCalled();
  });
});

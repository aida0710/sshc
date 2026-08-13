// 端末セッションひとつ分の通信路。
//
// **バイナリフレーム** が PTY の生バイト列で、サーバー→クライアントは出力、
// クライアント→サーバーは打鍵である。base64 を挟まない。**テキストフレーム**
// は JSON の制御メッセージで、いまは resize と exit の二つだけを運ぶ。
//
// この経路が `/api/` の外にあるのは、ブラウザが WebSocket のハンドシェイクに
// カスタムヘッダを付けられないからである。認可は直前の CSRF 付き要求が発行した
// 使い捨てのチケットひとつが行う。

export type ExitReport = { code: number; signal: string };

export type StreamHandlers = {
  onOutput: (chunk: Uint8Array) => void;
  onExit: (exit: ExitReport) => void;
  // onClose は、終了ではなく通信路が切れたときに呼ばれる。セッションは死んで
  // いないので、呼び出し側は繋ぎ直せる。
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

// openStream は WebSocket を開き、届いたフレームをハンドラへ渡す。
export function openStream(ticket: string, handlers: StreamHandlers): TerminalStream {
  const socket = new WebSocket(streamURL(ticket));
  socket.binaryType = "arraybuffer";
  const encoder = new TextEncoder();
  let exited = false;

  socket.addEventListener("message", (event: MessageEvent<unknown>) => {
    if (event.data instanceof ArrayBuffer) {
      handlers.onOutput(new Uint8Array(event.data));
      return;
    }
    if (typeof event.data !== "string") return;
    // 制御メッセージが壊れていても通信路は落とさない。読めないフレームひとつが
    // 生きているセッションを閉じてよい理由はない。
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
      if (socket.readyState !== WebSocket.OPEN) return;
      socket.send(encoder.encode(data));
    },
    resize(cols, rows) {
      if (socket.readyState !== WebSocket.OPEN) return;
      socket.send(JSON.stringify({ resize: { cols, rows } }));
    },
    close() {
      exited = true;
      socket.close();
    },
  };
}

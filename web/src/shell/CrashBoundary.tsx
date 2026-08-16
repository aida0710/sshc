import { Component, type CSSProperties, type ErrorInfo, type ReactNode } from "react";

// **真っ白な画面は、最悪の失敗の形である。** React は描画中の例外を捕まえる
// ものが無いとツリーごと外すので、残るのは空の body だけになる。何が起きたかを
// 知る手段は devtools しかなく、それを開けない機械——Android の WebView、
// 他人の端末——では、報告できることが「白かった」以外に何も無い。
//
// **ここは i18n の外に置く。** 文言を context から引くと、その context を
// 壊した例外をこの画面自身が踏む。訳されない英語が出ることより、何も出ない
// ことの方が悪い。
//
// **URL を出さない。** 入口の fragment を含み得る。出すのは例外の名前と
// メッセージ、そしてスタックの先頭数行だけである。

type State = { message: string; stack: string };

export class CrashBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { message: "", stack: "" };

  static getDerivedStateFromError(error: unknown): State {
    const failure = error instanceof Error ? error : new Error(String(error));
    return {
      message: `${failure.name}: ${failure.message}`,
      // 先頭の数行で足りる。全部出すと、読む人が最初の 1 行に辿り着けない。
      stack: (failure.stack ?? "").split("\n").slice(1, 6).join("\n"),
    };
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error("sshc stopped rendering", error, info.componentStack);
  }

  render() {
    if (this.state.message === "") return this.props.children;
    return (
      <main style={crashPage}>
        <h1 style={heading}>sshc stopped rendering</h1>
        <p style={note}>
          This is a defect. Copy the lines below into a report — they name the failure and
          nothing else.
        </p>
        <pre style={block}>{this.state.message}</pre>
        {this.state.stack === "" ? null : <pre style={block}>{this.state.stack}</pre>}
        <button
          type="button"
          style={action}
          onClick={() => window.location.replace(window.location.pathname + window.location.search)}
        >
          Reload
        </button>
      </main>
    );
  }
}

// スタイルはインラインである。**壊れたのがスタイルシートの読み込みだった場合、
// クラス名は何も塗らない。** この画面だけは、他の何にも依存せずに出る。
const crashPage: CSSProperties = {
  padding: "24px",
  paddingTop: "48px",
  display: "flex",
  flexDirection: "column",
  gap: "12px",
  alignItems: "flex-start",
  fontFamily: "system-ui, sans-serif",
  color: "#111",
  background: "#fff",
  minHeight: "100vh",
};
const heading: CSSProperties = { fontSize: "16px", fontWeight: 600, margin: 0 };
const note: CSSProperties = { fontSize: "13px", margin: 0, lineHeight: 1.5 };
const block: CSSProperties = {
  fontSize: "12px",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  background: "#f3f3f3",
  border: "1px solid #ddd",
  borderRadius: "6px",
  padding: "8px",
  margin: 0,
  maxWidth: "100%",
};
const action: CSSProperties = {
  fontSize: "14px",
  padding: "10px 16px",
  borderRadius: "6px",
  border: "1px solid #888",
  background: "#fff",
};

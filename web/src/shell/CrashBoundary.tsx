import { Component, type CSSProperties, type ErrorInfo, type ReactNode } from "react";
import { detectLocale, type Locale } from "../i18n/locale";

type State = { message: string; stack: string };

type CrashCopy = {
  heading: string;
  note: string;
  reload: string;
};

const crashCopy: Record<Locale, CrashCopy> = {
  en: {
    heading: "sshc stopped rendering",
    note: "This is a defect. Copy the lines below into a report. They contain only the failure details.",
    reload: "Reload",
  },
  ja: {
    heading: "sshc の画面を表示できませんでした",
    note: "アプリの不具合です。下の内容にはエラー情報だけが含まれています。コピーして不具合報告に添えてください。",
    reload: "再読み込み",
  },
};

function currentCrashCopy(): CrashCopy {
  try {
    return crashCopy[detectLocale()];
  } catch {
    return crashCopy.en;
  }
}

export class CrashBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { message: "", stack: "" };

  static getDerivedStateFromError(error: unknown): State {
    const failure = error instanceof Error ? error : new Error(String(error));
    return {
      message: `${failure.name}: ${failure.message}`,
      stack: (failure.stack ?? "").split("\n").slice(1, 6).join("\n"),
    };
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error("sshc stopped rendering", error, info.componentStack);
  }

  render() {
    if (this.state.message === "") return this.props.children;
    const copy = currentCrashCopy();
    return (
      <main style={crashPage}>
        <h1 style={heading}>{copy.heading}</h1>
        <p style={note}>{copy.note}</p>
        <pre style={block}>{this.state.message}</pre>
        {this.state.stack === "" ? null : <pre style={block}>{this.state.stack}</pre>}
        <button
          type="button"
          style={action}
          onClick={() => window.location.replace(window.location.pathname + window.location.search)}
        >
          {copy.reload}
        </button>
      </main>
    );
  }
}

const crashPage: CSSProperties = {
  padding: "24px",
  paddingTop: "48px",
  display: "flex",
  flexDirection: "column",
  gap: "12px",
  alignItems: "flex-start",
  fontFamily: "system-ui, sans-serif",
  color: "#111111", // palette-exempt
  background: "#ffffff", // palette-exempt
  minHeight: "100vh",
};
const heading: CSSProperties = { fontSize: "16px", fontWeight: 600, margin: 0 };
const note: CSSProperties = { fontSize: "13px", margin: 0, lineHeight: 1.5 };
const block: CSSProperties = {
  fontSize: "12px",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  background: "#f3f3f3", // palette-exempt
  border: "1px solid #dddddd", // palette-exempt
  borderRadius: "6px",
  padding: "8px",
  margin: 0,
  maxWidth: "100%",
};
const action: CSSProperties = {
  fontSize: "14px",
  padding: "10px 16px",
  borderRadius: "6px",
  border: "1px solid #888888", // palette-exempt
  background: "#ffffff", // palette-exempt
};

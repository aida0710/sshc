import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import {
  type IntegrationsApi,
  type OpenTerminalSessionRequest,
  type TerminalSession,
} from "../api/integrations";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

export type TerminalSessionsApi = Pick<
  IntegrationsApi,
  "terminalSessions" | "openTerminalSession" | "closeTerminalSession" | "renameTerminalSession"
>;

export type TerminalSessionsState = {
  sessions: TerminalSession[];
  maxSessions: number;
  busy: boolean;
  problem: string;
  // loaded は、最初の一覧が届いたかどうかである。ナビゲーションの既定の面は
  // 「セッションが 1 本でもあるか」で決まるので、まだ何も読んでいない状態と
  // 「読んだ結果 0 本だった」状態を区別できなければ、起動のたびに面が跳ねる。
  loaded: boolean;
  rename: (id: string, title: string) => Promise<boolean>;
  open: (request: OpenTerminalSessionRequest) => Promise<TerminalSession | null>;
  close: (id: string) => Promise<void>;
  refresh: () => Promise<void>;
  // markExited は、WebSocket が終了を告げた行を、一覧を取り直さずに描き直す。
  markExited: (id: string) => void;
};

// useTerminalSessions は、開いているセッションの一覧を保つ。
//
// PTY は常駐プロセス側で存続するので、これは正本ではなく写しである。リロード
// すれば同じ一覧が返り、開いていたセッションはそこにいる。
// enabled は、まだ読みに行ってはいけない状態を伝える。
//
// アプリケーションはマスターパスワードの向こう側にあるので、施錠中の /api/v1 は
// すべて vault_locked で拒否される。解錠より前に一覧を取りに行くと、その失敗が
// 「セッションは 0 本だった」として確定し、解錠後も誰も取り直さない。
export function useTerminalSessions(
  api: TerminalSessionsApi,
  translate: Translate,
  enabled = true,
): TerminalSessionsState {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [maxSessions, setMaxSessions] = useState(0);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    try {
      const listed = await api.terminalSessions();
      setSessions(listed.sessions);
      setMaxSessions(listed.maxSessions);
    } catch {
      // 一覧を読めないことは、開いている端末を失うことではない。次の操作で
      // また試すので、ここでは何も言わない。
    } finally {
      // 読めなかった場合も「読み終えた」と数える。届かない一覧を待ち続けて
      // ナビゲーションが面を決められないままになる方が悪い。
      setLoaded(true);
    }
  }, [api, enabled]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const open = useCallback(
    async (request: OpenTerminalSessionRequest): Promise<TerminalSession | null> => {
      setBusy(true);
      setProblem("");
      try {
        const opened = await api.openTerminalSession(request);
        await refresh();
        return opened.session;
      } catch (error) {
        // 「開けませんでした」だけでは、次に何をすればよいか分からない。
        // 設定そのものが接続を許さない場合は、その理由を名指しする。
        setProblem(translate(openFailureKey(failureCode(error))));
        return null;
      } finally {
        setBusy(false);
      }
    },
    [api, refresh, translate],
  );

  const close = useCallback(
    async (id: string) => {
      setBusy(true);
      try {
        const listed = await api.closeTerminalSession(id);
        setSessions(listed.sessions);
        setMaxSessions(listed.maxSessions);
      } catch {
        setProblem(translate("terminal.closeFailed"));
      } finally {
        setBusy(false);
      }
    },
    [api, translate],
  );

  // rename は表示だけを変える。走っているプロセスにも ssh の相手にも触れない。
  // 応答が一覧を返すので、それをそのまま写しに使う。
  const rename = useCallback(
    async (id: string, title: string): Promise<boolean> => {
      try {
        const listed = await api.renameTerminalSession(id, title);
        setSessions(listed.sessions);
        setMaxSessions(listed.maxSessions);
        return true;
      } catch {
        setProblem(translate("terminal.renameFailed"));
        return false;
      }
    },
    [api, translate],
  );

  const markExited = useCallback((id: string) => {
    setSessions((current) =>
      current.map((session) =>
        session.id === id && session.exited === undefined
          ? { ...session, exited: { code: 0, signal: "", at: "" } }
          : session,
      ),
    );
  }, []);

  return { sessions, maxSessions, busy, problem, loaded, rename, open, close, refresh, markExited };
}

// openFailureKey は、サーバーが名指しした理由を画面の文言へ移す。
//
// ここに無い符号は「開けませんでした」になる。**推測して言い換えない**——
// 知らない理由に説明を付けると、その説明は必ずいつか嘘になる。
function openFailureKey(code: string): MessageKey {
  switch (code) {
    case "terminal_session_limit":
      return "terminal.limitRefused";
    case "alias_unresolvable":
      return "terminal.unresolvable";
    case "proxy_command_refused":
      return "terminal.proxyCommandRefused";
    case "jump_depth_exceeded":
      return "terminal.jumpDepthExceeded";
    default:
      return "terminal.openFailed";
  }
}

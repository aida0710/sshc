import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import {
  type IntegrationsApi,
  type OpenTerminalSessionRequest,
  type TerminalSession,
} from "../api/integrations";
import type { Translate } from "../i18n/context";

export type TerminalSessionsApi = Pick<
  IntegrationsApi,
  "terminalSessions" | "openTerminalSession" | "closeTerminalSession"
>;

export type TerminalSessionsState = {
  sessions: TerminalSession[];
  maxSessions: number;
  busy: boolean;
  problem: string;
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
export function useTerminalSessions(
  api: TerminalSessionsApi,
  translate: Translate,
): TerminalSessionsState {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [maxSessions, setMaxSessions] = useState(0);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");

  const refresh = useCallback(async () => {
    try {
      const listed = await api.terminalSessions();
      setSessions(listed.sessions);
      setMaxSessions(listed.maxSessions);
    } catch {
      // 一覧を読めないことは、開いている端末を失うことではない。次の操作で
      // また試すので、ここでは何も言わない。
    }
  }, [api]);

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
        // 上限に達したことと、開けなかったことは別の答えである。前者は
        // どれかを閉じれば直り、それが画面の言えることの中でいちばん役に立つ。
        setProblem(
          failureCode(error) === "terminal_session_limit"
            ? translate("terminal.limitRefused")
            : translate("terminal.openFailed"),
        );
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

  const markExited = useCallback((id: string) => {
    setSessions((current) =>
      current.map((session) =>
        session.id === id && session.exited === undefined
          ? { ...session, exited: { code: 0, signal: "", at: "" } }
          : session,
      ),
    );
  }, []);

  return { sessions, maxSessions, busy, problem, open, close, refresh, markExited };
}

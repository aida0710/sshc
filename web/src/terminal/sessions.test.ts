import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { useTerminalSessions, type TerminalSessionsApi } from "./sessions";

const list = { sessions: [], maxSessions: 50 };

function api(overrides: Partial<TerminalSessionsApi> = {}): TerminalSessionsApi {
  return {
    terminalSessions: vi.fn().mockResolvedValue(list),
    openTerminalSession: vi.fn(),
    closeTerminalSession: vi.fn().mockResolvedValue(list),
    renameTerminalSession: vi.fn().mockResolvedValue(list),
    ...overrides,
  } as TerminalSessionsApi;
}

// 英語のカタログを通さずに検査するので、翻訳はキーをそのまま返す。
const translate = ((key: string) => key) as never;

describe("useTerminalSessions", () => {
  // 上限に達したことと、開けなかったことは別の答えである。前者はどれかを
  // 閉じれば直り、それが画面の言えることの中でいちばん役に立つ。
  it("separates the session limit from a console that could not be opened", async () => {
    const openTerminalSession = vi
      .fn()
      .mockRejectedValueOnce(
        new ApiError("terminal_session_limit", 409, { code: "terminal_session_limit", message: "full" }),
      )
      .mockRejectedValueOnce(
        new ApiError("terminal_start_failed", 500, { code: "terminal_start_failed", message: "refused" }),
      );
    const { result } = renderHook(() => useTerminalSessions(api({ openTerminalSession }), translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.open({ kind: "shell" });
    });
    expect(result.current.problem).toBe("terminal.limitRefused");

    await act(async () => {
      await result.current.open({ kind: "shell" });
    });
    expect(result.current.problem).toBe("terminal.openFailed");
  });

  // 読めなかった一覧も「読み終えた」と数える。届かない一覧を待ち続けて
  // ナビゲーションが面を決められないままになる方が悪い。
  it("reports a finished load even when the list could not be read", async () => {
    const terminalSessions = vi.fn().mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useTerminalSessions(api({ terminalSessions }), translate));

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.sessions).toEqual([]);
  });

  // アプリケーションはマスターパスワードの向こう側にある。施錠中に読みに行くと
  // その失敗が「0 本だった」として確定し、解錠後も誰も取り直さない。
  it("reads nothing until it is allowed to", async () => {
    const terminalSessions = vi.fn().mockResolvedValue(list);
    const { result, rerender } = renderHook(
      ({ enabled }) => useTerminalSessions(api({ terminalSessions }), translate, enabled),
      { initialProps: { enabled: false } },
    );

    expect(terminalSessions).not.toHaveBeenCalled();
    expect(result.current.loaded).toBe(false);

    rerender({ enabled: true });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(terminalSessions).toHaveBeenCalled();
  });

  it("says so when a rename is refused and keeps the list it had", async () => {
    const renameTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("invalid_terminal_title", 400, { code: "invalid_terminal_title", message: "no" }),
    );
    const { result } = renderHook(() => useTerminalSessions(api({ renameTerminalSession }), translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    let outcome = true;
    await act(async () => {
      outcome = await result.current.rename("a", "  ");
    });

    expect(outcome).toBe(false);
    expect(result.current.problem).toBe("terminal.renameFailed");
  });
});

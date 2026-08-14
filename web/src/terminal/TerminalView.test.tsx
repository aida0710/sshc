import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StreamHandlers, TerminalStream } from "./stream";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/integrations";

// 通信路そのものは stream.test.ts が見ている。ここで見るのは、それが切れた
// あとに誰が繋ぎ直すかである。
const streams: { handlers: StreamHandlers; stream: TerminalStream }[] = [];
vi.mock("./stream", () => ({
  openStream: (_ticket: string, handlers: StreamHandlers) => {
    const stream: TerminalStream = { send: vi.fn(), resize: vi.fn(), close: vi.fn() };
    streams.push({ handlers, stream });
    return stream;
  },
}));

const { TerminalView } = await import("./TerminalView");

const session: TerminalSession = {
  id: "a", kind: "shell", title: "zsh", startedAt: "2026-08-14T09:00:00Z",
};

function renderView(terminalStreamTicket = vi.fn(async () => ({ streamTicket: "one-time" }))) {
  render(<TerminalView session={session} api={{ terminalStreamTicket }} />);
  return terminalStreamTicket;
}

beforeEach(() => {
  streams.length = 0;
  // jsdom には matchMedia も ResizeObserver もない。xterm は開かれたときに
  // 画面の解像度を尋ね（古い addListener で）、こちらは枠の大きさを見張る。
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia;
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

afterEach(() => {
  vi.useRealTimers();
});

describe("TerminalView", () => {
  // **通信路が切れることは、セッションが死ぬことではない。** PTY は常駐
  // プロセス側で生きているので、黙って諦めずに繋ぎ直す。そして繋ぎ直して
  // いることを言う——待たされている理由が読めなければ、壊れているのと同じである。
  it("says that it is retrying, counts down and reattaches on its own", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    vi.useFakeTimers({ shouldAdvanceTime: true });
    streams[0]!.handlers.onClose();

    expect(await screen.findByText(/Attempt 2 in 1s/)).toBeVisible();
    await vi.advanceTimersByTimeAsync(1000);
    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(streams).toHaveLength(2));
    // 繋がったら黙る。言うことがあるのは、繋がっていないあいだだけである。
    await waitFor(() => expect(screen.queryByText(/Attempt/)).toBeNull());
  });

  // 待っているあいだ、人は待たされる側でいなくてよい。
  it("connects at once when asked instead of waiting out the delay", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    streams[0]!.handlers.onClose();
    await userEvent.click(await screen.findByRole("button", { name: "Connect now" }));

    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
  });

  // 止めたら止まる。**勝手に繋ぎ直さない。** セッションは残っているので、
  // その気になったときに繋ぎ直せることも同じ場所が言う。
  it("stops retrying when told to, and offers the way back", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    vi.useFakeTimers({ shouldAdvanceTime: true });
    streams[0]!.handlers.onClose();
    await userEvent.click(await screen.findByRole("button", { name: "Stop retrying" }));

    expect(screen.getByText(/Retrying has stopped/)).toBeVisible();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(ticket).toHaveBeenCalledTimes(1);

    await userEvent.click(screen.getByRole("button", { name: "Connect now" }));
    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
  });

  // もう無いセッションへは繋ぎ直さない。待っても戻ってこないものを待たない。
  it("does not retry a session that is gone", async () => {
    const ticket = vi.fn(async () => {
      throw new ApiError("terminal_session_not_found", 404, null);
    });
    render(<TerminalView session={session} api={{ terminalStreamTicket: ticket }} />);

    expect(await screen.findByText(/This session is gone/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Connect now" })).toBeNull();
  });

  // 終わったセッションは切断ではない。終了は理由が読める終わり方であり、
  // そこへ繋ぎ直しに行くことは何の役にも立たない。
  it("does not retry after the program exited", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    streams[0]!.handlers.onExit({ code: 0, signal: "" });
    streams[0]!.handlers.onClose();

    expect(screen.queryByText(/Attempt/)).toBeNull();
    expect(ticket).toHaveBeenCalledTimes(1);
  });
});

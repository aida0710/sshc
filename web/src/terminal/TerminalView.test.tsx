import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StreamHandlers, TerminalStream } from "./stream";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/integrations";

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
  id: "a", kind: "shell", title: "zsh", startedAt: "2026-08-14T09:00:00Z", state: "connected", problem: "",
};

function renderView(terminalStreamTicket = vi.fn(async () => ({ streamTicket: "one-time" }))) {
  render(<TerminalView session={session} api={{ terminalStreamTicket }} />);
  return terminalStreamTicket;
}

beforeEach(() => {
  streams.length = 0;
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
  it("keeps its title and search toolbar visible", () => {
    render(<TerminalView session={session} api={{ terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);

    expect(screen.getByRole("region", { name: "Console for zsh" })).toBeVisible();
    expect(screen.getByText("zsh", { exact: true })).toBeVisible();
    expect(screen.getByRole("button", { name: "Find" })).toBeVisible();
  });

  it("says that it is retrying, counts down and reattaches on its own", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    vi.useFakeTimers({ shouldAdvanceTime: true });
    streams[0]!.handlers.onClose();

    expect(await screen.findByText(/Attempt 2 in 1s/)).toBeVisible();
    await vi.advanceTimersByTimeAsync(1000);
    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(streams).toHaveLength(2));
    await waitFor(() => expect(screen.queryByText(/Attempt/)).toBeNull());
  });

  it("connects at once when asked instead of waiting out the delay", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    streams[0]!.handlers.onClose();
    await userEvent.click(await screen.findByRole("button", { name: "Connect now" }));

    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
  });

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

  it("does not retry a session that is gone", async () => {
    const ticket = vi.fn(async () => {
      throw new ApiError("terminal_session_not_found", 404, null);
    });
    render(<TerminalView session={session} api={{ terminalStreamTicket: ticket }} />);

    expect(await screen.findByText(/session no longer exists/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "Connect now" })).toBeNull();
  });

  it("does not retry after the program exited", async () => {
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    streams[0]!.handlers.onExit({ code: 0, signal: "" });
    streams[0]!.handlers.onClose();

    expect(screen.queryByText(/Attempt/)).toBeNull();
    expect(ticket).toHaveBeenCalledTimes(1);
  });

  it("offers an explicit reconnect inside an exited SSH terminal and reattaches", async () => {
    const ticket = vi.fn(async () => ({ streamTicket: "one-time" }));
    const onReconnect = vi.fn().mockResolvedValue(true);
    render(<TerminalView session={{ ...session, kind: "ssh", alias: "bastion", title: "bastion", state: "exited", exited: { code: 255, signal: "", at: "2026-08-26T01:00:00Z" } }} api={{ terminalStreamTicket: ticket }} onReconnect={onReconnect} />);
    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(1));

    await userEvent.click(screen.getByRole("button", { name: "Reconnect" }));

    expect(onReconnect).toHaveBeenCalledOnce();
    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
  });

  it("keeps the reconnect action in the terminal when the attempt is refused", async () => {
    const onReconnect = vi.fn().mockResolvedValue(false);
    render(<TerminalView session={{ ...session, kind: "ssh", alias: "bastion", title: "bastion", state: "exited", exited: { code: 255, signal: "", at: "2026-08-26T01:00:00Z" } }} onReconnect={onReconnect} />);

    await userEvent.click(await screen.findByRole("button", { name: "Reconnect" }));

    expect(await screen.findByText(/could not be reconnected/)).toBeVisible();
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeEnabled();
  });

  it("does not offer SSH reconnect for an exited local shell", () => {
    render(<TerminalView session={{ ...session, state: "exited", exited: { code: 0, signal: "", at: "2026-08-26T01:00:00Z" } }} onReconnect={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Reconnect" })).toBeNull();
  });
});

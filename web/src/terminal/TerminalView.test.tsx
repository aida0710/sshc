import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Terminal } from "@xterm/xterm";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StreamHandlers, TerminalStream } from "./stream";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/terminalSessions";
import { reducedMotionQuery } from "../ui/reducedMotion";
import { mobileViewportQuery } from "../ui/useMediaQuery";

const streams: { handlers: StreamHandlers; stream: TerminalStream }[] = [];
vi.mock("./stream", () => ({
  openStream: (_ticket: string, _streamPath: string, handlers: StreamHandlers) => {
    const stream: TerminalStream = { send: vi.fn(), resize: vi.fn(), close: vi.fn() };
    streams.push({ handlers, stream });
    return stream;
  },
}));
vi.mock("./TerminalQuickCommands", () => ({
  TerminalQuickCommands: () => <div role="dialog" aria-label="Quick Commands panel">Server-backed commands</div>,
}));

const { TerminalView } = await import("./TerminalView");

const session: TerminalSession = {
  id: "a", kind: "shell", workspaceId: "default", title: "zsh", startedAt: "2026-08-14T09:00:00Z", state: "connected", problem: "",
};

function renderView(terminalStreamTicket = vi.fn(async () => ({ streamTicket: "one-time" }))) {
  render(<TerminalView session={session} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket }} />);
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
  }));
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
  it("says which VPN route an SSH connection takes, and says nothing when it takes none", () => {
    const ssh: TerminalSession = { ...session, kind: "ssh", alias: "tains", title: "tains" };
    const { rerender } = render(
      <TerminalView session={ssh} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} vpnProfile="tohoku" />,
    );

    expect(screen.getByText("VPN: tohoku")).toBeVisible();

    rerender(<TerminalView session={ssh} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);
    expect(screen.queryByText(/^VPN: /)).toBeNull();
  });

  it("retains normal cursor blinking when reduced motion is disabled", () => {
    let cursorBlink: boolean | undefined;
    const originalOpen = Terminal.prototype.open;
    const open = vi.spyOn(Terminal.prototype, "open").mockImplementation(function (this: Terminal, parent: HTMLElement) {
      cursorBlink = this.options.cursorBlink;
      return originalOpen.call(this, parent);
    });

    try {
      renderView();
      expect(cursorBlink).toBe(true);
    } finally {
      open.mockRestore();
    }
  });

  it("keeps the cursor static when the operating system requests reduced motion", () => {
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query === reducedMotionQuery,
      media: query,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    let cursorBlink: boolean | undefined;
    const originalOpen = Terminal.prototype.open;
    const open = vi.spyOn(Terminal.prototype, "open").mockImplementation(function (this: Terminal, parent: HTMLElement) {
      cursorBlink = this.options.cursorBlink;
      return originalOpen.call(this, parent);
    });

    try {
      renderView();
      expect(cursorBlink).toBe(false);
    } finally {
      open.mockRestore();
    }
  });

  it("uses the phone font size on a phone viewport and follows the viewport when it changes", () => {
    let phone = true;
    const listeners: (() => void)[] = [];
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      get matches() { return query === mobileViewportQuery && phone; },
      media: query,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: (_type: string, listener: () => void) => { listeners.push(listener); },
      removeEventListener: () => {},
    }));
    let view: Terminal | undefined;
    const originalOpen = Terminal.prototype.open;
    const open = vi.spyOn(Terminal.prototype, "open").mockImplementation(function (this: Terminal, parent: HTMLElement) {
      view = this;
      return originalOpen.call(this, parent);
    });

    try {
      renderView();
      expect(view?.options.fontSize).toBe(15);

      phone = false;
      act(() => listeners.forEach((listener) => listener()));
      expect(view?.options.fontSize).toBe(13);
    } finally {
      open.mockRestore();
    }
  });

  for (const [name, visibleText, opensAtOnce] of [
    ["shows the real address first when an OSC 8 link's text names another address", "https://github.com/", false],
    ["opens an OSC 8 link at once when its text names its own address", "https://evil.example/", true],
  ] as const) {
    it(name, async () => {
      let view: Terminal | undefined;
      const originalOpen = Terminal.prototype.open;
      const open = vi.spyOn(Terminal.prototype, "open").mockImplementation(function (this: Terminal, parent: HTMLElement) {
        view = this;
        return originalOpen.call(this, parent);
      });
      const openWindow = vi.spyOn(window, "open").mockReturnValue(null);

      try {
        renderView();
        const terminal = view!;
        // What `\e]8;;https://evil.example/\e\\<text>\e]8;;\e\\` leaves on screen.
        await new Promise<void>((resolve) => terminal.write(visibleText, resolve));
        const range = { start: { x: 1, y: 1 }, end: { x: visibleText.length, y: 1 } };
        const ctrlClick = new MouseEvent("click", { ctrlKey: true, clientX: 10, clientY: 10 });
        act(() => terminal.options.linkHandler?.activate(ctrlClick, "https://evil.example/", range));

        if (opensAtOnce) {
          expect(openWindow).toHaveBeenCalledWith("https://evil.example/", "_blank", "noopener,noreferrer");
          expect(screen.queryByRole("dialog", { name: "Terminal link actions" })).toBeNull();
        } else {
          expect(openWindow).not.toHaveBeenCalled();
          const popover = screen.getByRole("dialog", { name: "Terminal link actions" });
          expect(popover).toHaveTextContent("https://evil.example/");
        }
      } finally {
        openWindow.mockRestore();
        open.mockRestore();
      }
    });
  }

  it("keeps its title and search toolbar visible", () => {
    render(<TerminalView session={session} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);

    expect(screen.getByRole("region", { name: "Terminal for zsh" })).toBeVisible();
    expect(screen.getByText("zsh", { exact: true })).toBeVisible();
    expect(screen.getByRole("button", { name: "Find" })).toBeVisible();
    expect(screen.getByText("connected", { exact: true })).toBeVisible();
  });

  it("waits for a deliberate tap before focusing input on a touch device", async () => {
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query.includes("pointer: coarse"), media: query,
      addEventListener: vi.fn(), removeEventListener: vi.fn(),
      addListener: vi.fn(), removeListener: vi.fn(),
    }));
    const focus = vi.spyOn(Terminal.prototype, "focus");
    try {
      renderView();
      await waitFor(() => expect(streams).toHaveLength(1));
      expect(focus).not.toHaveBeenCalled();
      const overlay = document.querySelector(".sshc-select-overlay")!;
      fireEvent.touchStart(overlay, { touches: [{ clientX: 20, clientY: 20 }] });
      fireEvent.touchMove(overlay, { touches: [{ clientX: 20, clientY: 60 }] });
      fireEvent.touchEnd(overlay);
      expect(focus).not.toHaveBeenCalled();
      fireEvent.touchStart(overlay, { touches: [{ clientX: 20, clientY: 20 }] });
      fireEvent.touchEnd(overlay);
      expect(focus).toHaveBeenCalledOnce();
    } finally {
      focus.mockRestore();
    }
  });

  it("owns search from outside the terminal and repeated searches only while active", async () => {
    const api = { streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) };
    const view = render(<TerminalView session={session} api={api} searchShortcutActive />);
    expect(fireEvent.keyDown(document.body, { key: "f", ctrlKey: true, cancelable: true })).toBe(false);
    const input = await screen.findByRole("textbox", { name: "Search terminal output" });
    expect(input).toHaveFocus();
    fireEvent.change(input, { target: { value: "keep query" } });
    expect(fireEvent.keyDown(input, { key: "f", ctrlKey: true, cancelable: true })).toBe(false);
    expect(input).toHaveValue("keep query");
    view.rerender(<TerminalView session={session} api={api} searchShortcutActive={false} />);
    expect(fireEvent.keyDown(input, { key: "f", ctrlKey: true, cancelable: true })).toBe(true);
  });

  it("opens search as an overlay with match-case, regex and invalid-pattern feedback", async () => {
    renderView();
    await userEvent.click(screen.getByRole("button", { name: "Find" }));

    const input = screen.getByRole("textbox", { name: "Search terminal output" });
    expect(input.parentElement).toHaveClass("absolute");
    const matchCase = screen.getByRole("button", { name: "Match case" });
    const regex = screen.getByRole("button", { name: "Use regular expression" });
    expect(matchCase).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(matchCase);
    await userEvent.click(regex);
    expect(matchCase).toHaveAttribute("aria-pressed", "true");
    expect(regex).toHaveAttribute("aria-pressed", "true");
    fireEvent.change(input, { target: { value: "[" } });
    expect(await screen.findByText("Invalid")).toBeVisible();
  });

  it("accepts OSC 52 only after it is turned on for every terminal", async () => {
    renderView();
    await userEvent.click(screen.getByRole("button", { name: "More terminal actions" }));
    const toggle = screen.getByRole("menuitemcheckbox", { name: /OSC 52/ });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    await userEvent.click(toggle);
    expect(screen.getByText("OSC 52 clipboard writes are now allowed in every terminal.")).toBeVisible();
  });

  it("reports an OSC 52 change so the setting for every terminal is saved", async () => {
    const onOsc52Change = vi.fn();
    render(<TerminalView session={session} osc52Enabled onOsc52Change={onOsc52Change} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);
    await userEvent.click(screen.getByRole("button", { name: "More terminal actions" }));
    const toggle = screen.getByRole("menuitemcheckbox", { name: /OSC 52/ });
    expect(toggle).toHaveAttribute("aria-checked", "true");
    await userEvent.click(toggle);
    expect(onOsc52Change).toHaveBeenCalledWith(false);
  });

  it("keeps the OSC 52 toggle unchanged and reports persistence failure", async () => {
    const onOsc52Change = vi.fn(async () => Promise.reject(new Error("save failed")));
    render(<TerminalView session={session} osc52Enabled onOsc52Change={onOsc52Change} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);
    await userEvent.click(screen.getByRole("button", { name: "More terminal actions" }));
    await userEvent.click(screen.getByRole("menuitemcheckbox", { name: /OSC 52/ }));
    await waitFor(() => expect(onOsc52Change).toHaveBeenCalledWith(false));
    expect(await screen.findByText("The terminal settings could not be saved.")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "More terminal actions" }));
    expect(screen.getByRole("menuitemcheckbox", { name: /OSC 52/ })).toHaveAttribute("aria-checked", "true");
  });

  it("opens server-backed Quick Commands without wiring the browser stream writer", async () => {
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    await userEvent.click(screen.getByRole("button", { name: "More terminal actions" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Quick Commands" }));
    expect(screen.getByRole("dialog", { name: "Quick Commands panel" })).toBeVisible();
    expect(streams[0]?.stream.send).not.toHaveBeenCalled();
  });

  it("keeps only search and overflow in the terminal header", () => {
    renderView();
    expect(screen.getByRole("button", { name: "Find" })).toBeVisible();
    expect(screen.getByRole("button", { name: "More terminal actions" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Quick Commands" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copy recent terminal context" })).toBeNull();
    expect(within(screen.getByRole("button", { name: "Find" }).parentElement!).getAllByRole("button")).toHaveLength(2);
  });

  it("places additional actions in the existing header and removes them without reopening the terminal", async () => {
    const api = { streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) };
    const onAction = vi.fn();
    const view = render(<TerminalView session={session} api={api} headerActions={<button onClick={onAction}>Pane action</button>} />);
    await waitFor(() => expect(streams).toHaveLength(1));
    const header = screen.getByRole("button", { name: "Find" }).parentElement!;
    expect(within(header).getByText("zsh", { exact: true })).toBeVisible();
    await userEvent.click(within(header).getByRole("button", { name: "Pane action" }));
    expect(onAction).toHaveBeenCalledOnce();

    view.rerender(<TerminalView session={session} api={api} />);
    expect(screen.queryByRole("button", { name: "Pane action" })).toBeNull();
    expect(streams).toHaveLength(1);
    expect(streams[0]!.stream.close).not.toHaveBeenCalled();
  });

  it("reviews a multiline paste before sending it and can remove only the final Enter", async () => {
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;

    fireEvent.paste(host, {
      clipboardData: { getData: () => "printf one\nprintf two\n" },
    });

    expect(await screen.findByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();
    expect(streams[0]!.stream.send).not.toHaveBeenCalled();
    await userEvent.click(screen.getByText("Paste preview with control characters made visible"));
    expect(screen.getByLabelText("Paste preview with control characters made visible")).toHaveTextContent(
      "printf one\\n printf two\\n",
      { normalizeWhitespace: true },
    );

    await userEvent.click(screen.getByRole("button", { name: "Paste without final Enter" }));

    expect(streams[0]!.stream.send).toHaveBeenCalledOnce();
    expect(streams[0]!.stream.send).toHaveBeenCalledWith("printf one\rprintf two");
    expect(screen.queryByRole("dialog", { name: "Review paste to localhost" })).toBeNull();
  });

  describe("the phone's text box", () => {
    beforeEach(() => {
      window.matchMedia = vi.fn().mockImplementation((query: string) => ({
        matches: query === mobileViewportQuery,
        media: query,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
      }));
    });

    async function openComposer() {
      renderView();
      await waitFor(() => expect(streams).toHaveLength(1));
      const field = screen.getByRole("textbox", { name: "Text to send to the terminal" });
      await waitFor(() => expect(field).toBeEnabled());
      return field;
    }

    it("sends the typed text as a paste and then presses Enter, and empties itself", async () => {
      const field = await openComposer();

      await userEvent.type(field, "printf ok");
      await userEvent.click(screen.getByRole("button", { name: "Send" }));

      expect(streams[0]!.stream.send).toHaveBeenNthCalledWith(1, "printf ok");
      expect(streams[0]!.stream.send).toHaveBeenNthCalledWith(2, "\r");
      expect(field).toHaveValue("");
    });

    it("presses Enter when sent empty", async () => {
      await openComposer();

      await userEvent.click(screen.getByRole("button", { name: "Send" }));

      expect(streams[0]!.stream.send).toHaveBeenCalledOnce();
      expect(streams[0]!.stream.send).toHaveBeenCalledWith("\r");
    });

    it("reviews text with a line break like a paste, and sends it with Enter only when accepted", async () => {
      const field = await openComposer();
      fireEvent.change(field, { target: { value: "printf one\nprintf two" } });

      await userEvent.click(screen.getByRole("button", { name: "Send" }));
      expect(await screen.findByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();
      expect(streams[0]!.stream.send).not.toHaveBeenCalled();

      await userEvent.click(screen.getByRole("button", { name: "Paste" }));

      expect(streams[0]!.stream.send).toHaveBeenNthCalledWith(1, "printf one\rprintf two");
      expect(streams[0]!.stream.send).toHaveBeenNthCalledWith(2, "\r");
      expect(field).toHaveValue("");
    });

    it("keeps the text when the review is cancelled", async () => {
      const field = await openComposer();
      fireEvent.change(field, { target: { value: "printf one\nprintf two" } });

      await userEvent.click(screen.getByRole("button", { name: "Send" }));
      await userEvent.click(await screen.findByRole("button", { name: "Cancel" }));

      expect(streams[0]!.stream.send).not.toHaveBeenCalled();
      expect(field).toHaveValue("printf one\nprintf two");
    });
  });

  it("names the paste destination as the header does, the alias and not the title the terminal set", async () => {
    render(<TerminalView session={{
      ...session, kind: "ssh", alias: "osaka", title: "vim notes.md",
      presentation: { displayTitle: "vim notes.md", titleSource: "terminal", titlePinned: false },
    }} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;

    fireEvent.paste(host, { clipboardData: { getData: () => "echo one\necho two" } });

    expect(await screen.findByRole("dialog", { name: "Review paste to osaka" })).toBeVisible();
  });

  it("cancels a risky paste without sending any bytes", async () => {
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;
    fireEvent.paste(host, { clipboardData: { getData: () => "echo safe\necho dangerous" } });

    await userEvent.click(await screen.findByRole("button", { name: "Cancel" }));

    expect(streams[0]!.stream.send).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog", { name: "Review paste to localhost" })).toBeNull();
  });

  it("never carries a pending paste into a different terminal session", async () => {
    const ticket = vi.fn(async () => ({ streamTicket: "one-time" }));
    const rendered = render(<TerminalView session={session} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: ticket }} />);
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;
    fireEvent.paste(host, { clipboardData: { getData: () => "echo one\necho two" } });
    expect(await screen.findByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();

    rendered.rerender(
      <TerminalView
        session={{ ...session, id: "b", title: "bash" }}
        api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: ticket }}
      />,
    );

    expect(screen.queryByRole("dialog", { name: /Review paste to/ })).toBeNull();
    expect(streams[0]!.stream.send).not.toHaveBeenCalled();
  });

  it("keeps a pending paste while the program in the terminal redraws", async () => {
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;
    fireEvent.paste(host, { clipboardData: { getData: () => "echo one\necho two" } });
    expect(await screen.findByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();

    streams[0]!.handlers.onOutput(new TextEncoder().encode("\x1b[2J\x1b[H⠋ Thinking…"));

    expect(screen.getByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();
    expect(streams[0]!.stream.send).not.toHaveBeenCalled();
  });

  it("invalidates a pending paste when a reconnect replaces the shell", async () => {
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;
    fireEvent.paste(host, { clipboardData: { getData: () => "echo one\necho two" } });
    expect(await screen.findByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();

    act(() => streams[0]!.handlers.onShellReplaced());

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Review paste to localhost" })).toBeNull());
    expect(streams[0]!.stream.send).not.toHaveBeenCalled();
  });

  it("invalidates a pending paste when the connection to the terminal drops", async () => {
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;
    fireEvent.paste(host, { clipboardData: { getData: () => "echo one\necho two" } });
    expect(await screen.findByRole("dialog", { name: "Review paste to localhost" })).toBeVisible();

    act(() => streams[0]!.handlers.onClose());

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Review paste to localhost" })).toBeNull());
    expect(streams[0]!.stream.send).not.toHaveBeenCalled();
  });

  it("opens the working directory announced through OSC 7 in SFTP", async () => {
    const onOpenRemotePath = vi.fn();
    render(<TerminalView session={{
      ...session,
      kind: "ssh",
      alias: "bastion",
      title: "bastion",
    }} onOpenRemotePath={onOpenRemotePath} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);
    await waitFor(() => expect(streams).toHaveLength(1));
    streams[0]!.handlers.onReplay({ start: 0, next: 0, end: 0, truncated: false });
    streams[0]!.handlers.onOutput(new TextEncoder().encode("\x1b]7;file://bastion/srv/app\x07"));

    await userEvent.click(screen.getByRole("button", { name: "More terminal actions" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "Open current directory in SFTP" })).toBeEnabled());
    await userEvent.click(screen.getByRole("menuitem", { name: "Open current directory in SFTP" }));

    expect(onOpenRemotePath).toHaveBeenCalledWith("bastion", "/srv/app", "browse");
  });

  it("shows the current SSH hop and connection phase in the header", () => {
    render(<TerminalView session={{
      ...session, kind: "ssh", alias: "destination", title: "destination", state: "connecting",
      progress: {
        phase: "host_key", alias: "bastion", hostName: "192.0.2.10", user: "ops", hop: 1, hops: 2,
      },
    }} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);

    expect(screen.getByText("checking the host key for bastion · 1/2")).toBeVisible();
  });

  it("lets the user stop the automatic reconnect while it is waiting", async () => {
    const onStopReconnect = vi.fn().mockResolvedValue(true);
    render(<TerminalView session={{
      ...session, kind: "ssh", alias: "tv", title: "tv", state: "reconnecting",
      reconnect: { attempt: 2, limit: 5, retryAt: "2026-09-18T00:00:10Z", problem: "reconnect_failed" },
    }} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} onStopReconnect={onStopReconnect} />);

    expect(screen.getByText("reconnecting 2/5 · connecting")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Stop reconnecting" }));
    expect(onStopReconnect).toHaveBeenCalledOnce();
  });

  it("shows the title the terminal set together with the alias, without SSH noise", () => {
    render(<TerminalView session={{
      ...session, kind: "ssh", alias: "osaka", title: "vim notes.md",
      presentation: { displayTitle: "vim notes.md", titleSource: "terminal", titlePinned: false },
    }} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: vi.fn(async () => ({ streamTicket: "one-time" })) }} />);

    expect(screen.getByText("vim notes.md")).toBeVisible();
    expect(screen.getByText("osaka")).toBeVisible();
    expect(screen.getByText("connected")).toBeVisible();
    expect(screen.queryByText(/SSH ·/)).toBeNull();
  });

  it("says that it is retrying, counts down and reattaches on its own", async () => {
    const reset = vi.spyOn(Terminal.prototype, "reset");
    const ticket = renderView();
    await waitFor(() => expect(streams).toHaveLength(1));
    streams[0]!.handlers.onReplay({ start: 0, next: 0, end: 0, truncated: false });
    streams[0]!.handlers.onOutput(new TextEncoder().encode("first"));
    expect(reset).not.toHaveBeenCalled();

    vi.useFakeTimers({ shouldAdvanceTime: true });
    streams[0]!.handlers.onClose();

    expect(await screen.findByText(/Attempt 2 in 1s/)).toBeVisible();
    await vi.advanceTimersByTimeAsync(1000);
    await waitFor(() => expect(ticket).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(streams).toHaveLength(2));
    expect(ticket).toHaveBeenLastCalledWith("a", 5);
    expect(reset).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByText(/Attempt/)).toBeNull());
  });

  it("warns and restarts byte decoding when detached output exceeded the replay buffer", async () => {
    const write = vi.spyOn(Terminal.prototype, "write");
    renderView();
    await waitFor(() => expect(streams).toHaveLength(1));

    streams[0]!.handlers.onReplay({ start: 4096, next: 4096, end: 4096, truncated: true });

    expect(await screen.findByText(/no longer in the engine replay buffer/)).toBeVisible();
    expect(write).toHaveBeenCalledWith("\u0018");
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
    render(<TerminalView session={session} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: ticket }} />);

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
    render(<TerminalView session={{ ...session, kind: "ssh", alias: "bastion", title: "bastion", state: "exited", exited: { code: 255, signal: "", at: "2026-08-26T01:00:00Z" } }} api={{ streamPath: "/terminal/stream", terminalSessionDirectory: async () => null, terminalStreamTicket: ticket }} onReconnect={onReconnect} />);
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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { StreamHandlers, TerminalStream } from "./stream";
import { createStreamLink, stableStreamResetMs, type StreamLink } from "./streamLink";

const streams: { handlers: StreamHandlers; stream: TerminalStream }[] = [];
vi.mock("./stream", () => ({
  openStream: (_ticket: string, _streamPath: string, handlers: StreamHandlers) => {
    const stream: TerminalStream = { send: vi.fn(), sendBinary: vi.fn(), resize: vi.fn(), close: vi.fn() };
    streams.push({ handlers, stream });
    return stream;
  },
}));

type TicketIssuer = (id: string, cursor?: number) => Promise<{ streamTicket: string }>;

// Every link watches the page for resumes until it is closed, so each test's links are closed after it.
const started: { close: () => void }[] = [];

function startLink(terminalStreamTicket: TicketIssuer = async () => ({ streamTicket: "one-time" })) {
  const links: StreamLink[] = [];
  const ticket = vi.fn(terminalStreamTicket);
  const onExit = vi.fn();
  const onOutput = vi.fn();
  const onClose = vi.fn();
  const controller = createStreamLink({
    sessionId: "a",
    api: { streamPath: "/terminal/stream", terminalStreamTicket: ticket },
    onLink: (link) => links.push(link),
    onAttached: vi.fn(),
    onReplay: vi.fn(),
    onOutput,
    onExit,
    onShellReplaced: vi.fn(),
    onClose,
  });
  controller.connect();
  started.push(controller);
  return { controller, links, ticket, onExit, onOutput, onClose };
}

function lastLink(links: StreamLink[]): StreamLink | undefined {
  return links[links.length - 1];
}

// Seconds shown when each wait began, in order.
function waitsStarted(links: StreamLink[]): number[] {
  return links.flatMap((link, index) => {
    if (link.phase !== "waiting") return [];
    const previous = links[index - 1];
    return previous?.phase === "waiting" ? [] : [link.seconds];
  });
}

beforeEach(() => {
  streams.length = 0;
  vi.useFakeTimers();
});

afterEach(() => {
  for (const controller of started.splice(0)) controller.close();
  vi.useRealTimers();
});

describe("createStreamLink", () => {
  it("waits 1, 2, 4, 8 and then 15 seconds between attempts while attaching keeps failing", async () => {
    const { links, ticket } = startLink(async () => {
      throw new Error("offline");
    });

    await vi.advanceTimersByTimeAsync((1 + 2 + 4 + 8 + 15 + 15) * 1000);

    expect(waitsStarted(links)).toEqual([1, 2, 4, 8, 15, 15, 15]);
    expect(ticket).toHaveBeenCalledTimes(7);
  });

  it("counts down each second and names the attempt it is waiting for", async () => {
    const { links } = startLink(async () => {
      throw new Error("offline");
    });
    await vi.advanceTimersByTimeAsync(1000);

    links.length = 0;
    await vi.advanceTimersByTimeAsync(1000);

    expect(links).toEqual([{ phase: "waiting", attempt: 3, seconds: 1 }]);
  });

  it("keeps escalating the wait when a link drops soon after attaching", async () => {
    const { links } = startLink();
    await vi.advanceTimersByTimeAsync(0);

    streams[0]!.handlers.onClose();
    await vi.advanceTimersByTimeAsync(1000);
    streams[1]!.handlers.onClose();

    expect(waitsStarted(links)).toEqual([1, 2]);
  });

  it("starts the wait over from 1 second after a link that stayed up past the stable period", async () => {
    const { links } = startLink();
    await vi.advanceTimersByTimeAsync(0);
    streams[0]!.handlers.onClose();
    await vi.advanceTimersByTimeAsync(1000);

    await vi.advanceTimersByTimeAsync(stableStreamResetMs + 1);
    streams[1]!.handlers.onClose();

    expect(waitsStarted(links)).toEqual([1, 1]);
  });

  it("resumes from the byte after the last output it received", async () => {
    const { ticket } = startLink();
    await vi.advanceTimersByTimeAsync(0);

    streams[0]!.handlers.onReplay({ start: 100, next: 100, end: 100, truncated: false });
    streams[0]!.handlers.onOutput(new TextEncoder().encode("hello"));
    streams[0]!.handlers.onClose();
    await vi.advanceTimersByTimeAsync(1000);

    expect(ticket).toHaveBeenLastCalledWith("a", 105);
  });

  it("stops retrying on request and attaches again only when asked", async () => {
    const { controller, links, ticket } = startLink();
    await vi.advanceTimersByTimeAsync(0);
    streams[0]!.handlers.onClose();

    controller.stopRetrying();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(lastLink(links)).toEqual({ phase: "stopped", gone: false });
    expect(ticket).toHaveBeenCalledTimes(1);

    controller.connect();
    await vi.advanceTimersByTimeAsync(0);
    expect(ticket).toHaveBeenCalledTimes(2);
    expect(lastLink(links)).toEqual({ phase: "live" });
  });

  it("reports a session that no longer exists as gone and does not retry", async () => {
    const { links, ticket } = startLink(async () => {
      throw new ApiError("terminal_session_not_found", 404, null);
    });

    await vi.advanceTimersByTimeAsync(60_000);

    expect(lastLink(links)).toEqual({ phase: "stopped", gone: true });
    expect(ticket).toHaveBeenCalledTimes(1);
  });

  it("does not retry after the program exited", async () => {
    const { links, ticket, onExit } = startLink();
    await vi.advanceTimersByTimeAsync(0);

    streams[0]!.handlers.onExit({ code: 0, signal: "" });
    streams[0]!.handlers.onClose();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(onExit).toHaveBeenCalledOnce();
    expect(lastLink(links)).toEqual({ phase: "live" });
    expect(ticket).toHaveBeenCalledTimes(1);
  });

  it("attaches at once when the page comes back from the back-forward cache instead of waiting out the delay", async () => {
    const { links, ticket } = startLink();
    await vi.advanceTimersByTimeAsync(0);
    streams[0]!.handlers.onClose();
    expect(lastLink(links)).toMatchObject({ phase: "waiting" });

    window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
    await vi.advanceTimersByTimeAsync(0);

    expect(ticket).toHaveBeenCalledTimes(2);
    expect(lastLink(links)).toEqual({ phase: "live" });
  });

  it("attaches at once when the network comes back", async () => {
    let online = false;
    const { ticket } = startLink(async () => {
      if (!online) throw new Error("offline");
      return { streamTicket: "one-time" };
    });
    await vi.advanceTimersByTimeAsync(0);

    online = true;
    window.dispatchEvent(new Event("online"));
    await vi.advanceTimersByTimeAsync(0);

    expect(ticket).toHaveBeenCalledTimes(2);
    expect(streams).toHaveLength(1);
  });

  it("attaches a live link afresh when the network comes back, since the old one may lead nowhere", async () => {
    const { links, ticket } = startLink();
    await vi.advanceTimersByTimeAsync(0);
    const first = streams[0]!;

    window.dispatchEvent(new Event("online"));
    await vi.advanceTimersByTimeAsync(0);

    expect(first.stream.close).toHaveBeenCalledOnce();
    expect(ticket).toHaveBeenCalledTimes(2);
    expect(streams).toHaveLength(2);
    expect(links).toContainEqual({ phase: "connecting", attempt: 1 });
    expect(lastLink(links)).toEqual({ phase: "live" });
  });

  it("keeps typing into the live stream while it is attached afresh, and reads what it missed from the new one", async () => {
    let issue: (value: { streamTicket: string }) => void = () => {};
    const tickets = [Promise.resolve({ streamTicket: "first" }), new Promise<{ streamTicket: string }>((resolve) => { issue = resolve; })];
    const { controller, links, onOutput, onClose } = startLink(() => tickets.shift()!);
    await vi.advanceTimersByTimeAsync(0);
    const first = streams[0]!;
    first.handlers.onOutput(new Uint8Array([1, 2, 3]));
    const linksBefore = links.length;

    window.dispatchEvent(new Event("online"));
    await vi.advanceTimersByTimeAsync(0);

    // Until the new stream is attached, the old one takes input and the screen keeps showing it live.
    expect(controller.currentStream()).toBe(first.stream);
    expect(links.slice(linksBefore)).toEqual([]);
    // What the old stream receives now is read again from the new one, so it is not shown twice.
    first.handlers.onOutput(new Uint8Array([4]));
    expect(onOutput).toHaveBeenCalledTimes(1);

    issue({ streamTicket: "second" });
    await vi.advanceTimersByTimeAsync(0);

    expect(controller.currentStream()).toBe(streams[1]!.stream);
    expect(first.stream.close).toHaveBeenCalledOnce();
    first.handlers.onClose();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("keeps a stopped link stopped when the page or the network comes back", async () => {
    const { controller, ticket } = startLink(async () => {
      throw new Error("offline");
    });
    await vi.advanceTimersByTimeAsync(0);
    controller.stopRetrying();

    window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
    window.dispatchEvent(new Event("online"));
    await vi.advanceTimersByTimeAsync(0);

    expect(ticket).toHaveBeenCalledTimes(1);
  });

  it("does not attach again for a page shown for the first time", async () => {
    const { ticket } = startLink(async () => {
      throw new Error("offline");
    });
    await vi.advanceTimersByTimeAsync(0);

    window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: false }));
    await vi.advanceTimersByTimeAsync(0);

    expect(ticket).toHaveBeenCalledTimes(1);
  });

  it("closes the stream and abandons a pending wait when closed", async () => {
    const { controller, ticket } = startLink();
    await vi.advanceTimersByTimeAsync(0);
    const first = streams[0]!;

    controller.close();
    first.handlers.onClose();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(first.stream.close).toHaveBeenCalledOnce();
    expect(ticket).toHaveBeenCalledTimes(1);
  });
});

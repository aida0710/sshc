import { failureCode } from "../api/client";
import type { TerminalSessionsApi } from "../api/terminalSessions";
import { openStream, type StreamHandlers, type TerminalStream } from "./stream";

// Where the browser stands with the WebSocket behind a terminal. This is
// separate from the engine-side session state, which the session reports:
// a connected session can still have a dropped link while the tab was asleep.
export type StreamLink =
  | { phase: "live" }
  | { phase: "connecting"; attempt: number }
  | { phase: "waiting"; attempt: number; seconds: number }
  | { phase: "stopped"; gone: boolean };

// How long to wait before reattaching the browser's WebSocket after the 1st,
// 2nd, 3rd... drop in a row. This is only the browser-to-engine link; the
// engine's own SSH reconnect keeps a separate schedule (docs/design.md).
export const streamReconnectBackoffSeconds = [1, 2, 4, 8, 15];

// A link that stayed up this long counts as healthy, so its next drop starts
// the backoff from the first step instead of continuing to escalate.
export const stableStreamResetMs = 10_000;

// The waiting banner counts down in whole seconds.
const countdownTickMs = 1000;

export type StreamLinkOptions = StreamHandlers & {
  sessionId: string;
  api: Pick<TerminalSessionsApi, "terminalStreamTicket">;
  // Receives every phase change so the view can show where the link stands.
  onLink: (link: StreamLink) => void;
  // Runs each time a new stream is attached, before any output arrives.
  onAttached: () => void;
};

export type StreamLinkController = {
  // Attaches now: the first time, and again when the user does not want to
  // wait out the delay or wants retrying back after stopping it.
  connect: () => void;
  stopRetrying: () => void;
  currentStream: () => TerminalStream | null;
  close: () => void;
};

// createStreamLink keeps one terminal's WebSocket attached: it resumes from
// the last byte it received and, when the link drops, waits with backoff and
// attaches again until the program exits, the session is gone or the user
// stops it.
export function createStreamLink(options: StreamLinkOptions): StreamLinkController {
  let stream: TerminalStream | null = null;
  let closed = false;
  let stopped = false;
  let attempts = 0;
  let attachedAt = 0;
  let countdown: ReturnType<typeof setInterval> | undefined;
  let cursor: number | undefined;

  const halted = () => closed || stopped;

  const connect = () => {
    clearInterval(countdown);
    stopped = false;
    attempts += 1;
    options.onLink({ phase: "connecting", attempt: attempts });
    void options.api
      .terminalStreamTicket(options.sessionId, cursor)
      .then((issued) => {
        if (halted()) return;
        attachedAt = Date.now();
        stream = openStream(issued.streamTicket, {
          onReplay: (replay) => {
            cursor = replay.start;
            options.onReplay(replay);
          },
          onOutput: (chunk) => {
            options.onOutput(chunk);
            cursor = (cursor ?? 0) + chunk.byteLength;
          },
          onExit: (exit) => {
            stopped = true;
            options.onLink({ phase: "live" });
            options.onExit(exit);
          },
          onClose: () => {
            stream = null;
            options.onClose();
            waitAndReconnect();
          },
        });
        options.onLink({ phase: "live" });
        options.onAttached();
      })
      .catch((error: unknown) => {
        if (halted()) return;
        if (failureCode(error) === "terminal_session_not_found") {
          options.onLink({ phase: "stopped", gone: true });
          return;
        }
        waitAndReconnect();
      });
  };

  const waitAndReconnect = () => {
    if (halted()) return;
    if (attachedAt !== 0 && Date.now() - attachedAt > stableStreamResetMs) attempts = 1;
    const step = Math.min(attempts - 1, streamReconnectBackoffSeconds.length - 1);
    let secondsLeft = streamReconnectBackoffSeconds[step] ?? 1;
    const nextAttempt = attempts + 1;
    options.onLink({ phase: "waiting", attempt: nextAttempt, seconds: secondsLeft });
    countdown = setInterval(() => {
      secondsLeft -= 1;
      if (secondsLeft > 0) {
        options.onLink({ phase: "waiting", attempt: nextAttempt, seconds: secondsLeft });
        return;
      }
      connect();
    }, countdownTickMs);
  };

  return {
    connect,
    stopRetrying: () => {
      stopped = true;
      clearInterval(countdown);
      options.onLink({ phase: "stopped", gone: false });
    },
    currentStream: () => stream,
    close: () => {
      closed = true;
      clearInterval(countdown);
      stream?.close();
    },
  };
}

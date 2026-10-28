import { failureCode } from "../api/client";
import type { TerminalSessionsApi } from "../api/terminalSessions";
import { whenPageResumes } from "./pageResume";
import { openStream, type ReplayReport, type StreamHandlers, type TerminalStream } from "./stream";

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

export type StreamLinkOptions = Omit<StreamHandlers, "onReplay"> & {
  // resumed is true when the stream continues from what this view had already
  // shown, after the link was offline; false for a view that starts afresh
  // (a session shown again, a reloaded page), whose screen begins at the replay.
  onReplay: (replay: ReplayReport, resumed: boolean) => void;
  sessionId: string;
  api: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath">;
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
// stops it. When the page comes back from being away (see whenPageResumes), a
// wait is cut short and a link that still looks live is attached afresh: after
// a sleep it may lead nowhere, and attaching again resumes from the same byte.
export function createStreamLink(options: StreamLinkOptions): StreamLinkController {
  let stream: TerminalStream | null = null;
  // replacing is a live stream being attached afresh. It takes what is typed until the new stream is
  // attached, so nothing typed in between is dropped, and is closed then.
  let replacing: TerminalStream | null = null;
  let closed = false;
  let stopped = false;
  let attempts = 0;
  let attachedAt = 0;
  let countdown: ReturnType<typeof setInterval> | undefined;
  let cursor: number | undefined;

  const halted = () => closed || stopped;

  // muted holds the streams whose output is no longer wanted: a stream being replaced stops counting the
  // bytes it receives, and the new stream reads them again from the same place.
  const muted = new WeakSet<object>();

  const releaseReplaced = () => {
    const stale = replacing;
    replacing = null;
    stale?.close();
  };

  // connect attaches a stream. quiet is for attaching afresh a link that looks live: the screen keeps
  // showing it as live meanwhile.
  const connect = (quiet = false) => {
    clearInterval(countdown);
    waiting = false;
    stopped = false;
    attempts += 1;
    if (!quiet) options.onLink({ phase: "connecting", attempt: attempts });
    const resumeFrom = cursor;
    void options.api
      .terminalStreamTicket(options.sessionId, resumeFrom)
      .then((issued) => {
        if (halted()) return;
        attachedAt = Date.now();
        const attached: TerminalStream = openStream(issued.streamTicket, options.api.streamPath, {
          onReplay: (replay) => {
            if (muted.has(attached)) return;
            cursor = replay.start;
            options.onReplay(replay, resumeFrom !== undefined);
          },
          onOutput: (chunk) => {
            if (muted.has(attached)) return;
            options.onOutput(chunk);
            cursor = (cursor ?? 0) + chunk.byteLength;
          },
          onShellReplaced: () => {
            if (!muted.has(attached)) options.onShellReplaced();
          },
          onExit: (exit) => {
            if (muted.has(attached)) return;
            stopped = true;
            options.onLink({ phase: "live" });
            options.onExit(exit);
          },
          onClose: () => {
            if (muted.has(attached)) return;
            stream = null;
            options.onClose();
            waitAndReconnect();
          },
        });
        stream = attached;
        releaseReplaced();
        options.onLink({ phase: "live" });
        options.onAttached();
      })
      .catch((error: unknown) => {
        // The stream being replaced could not be attached afresh, so it most likely leads nowhere.
        releaseReplaced();
        if (halted()) return;
        if (failureCode(error) === "terminal_session_not_found") {
          options.onLink({ phase: "stopped", gone: true });
          return;
        }
        waitAndReconnect();
      });
  };

  let waiting = false;

  const waitAndReconnect = () => {
    if (halted()) return;
    waiting = true;
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

  // A link the user stopped stays stopped, and one being attached is left to finish.
  const attachAfresh = () => {
    if (halted()) return;
    if (waiting) {
      connect();
      return;
    }
    if (stream === null) return;
    // The live stream keeps taking what is typed until the new one is attached. Replacing it on purpose
    // does not count as a drop: the next attempt is the first again.
    muted.add(stream);
    replacing = stream;
    stream = null;
    attempts = 0;
    connect(true);
  };
  const stopWatchingResumes = whenPageResumes(attachAfresh);

  return {
    connect,
    stopRetrying: () => {
      stopped = true;
      waiting = false;
      clearInterval(countdown);
      options.onLink({ phase: "stopped", gone: false });
    },
    currentStream: () => stream ?? replacing,
    close: () => {
      closed = true;
      clearInterval(countdown);
      stopWatchingResumes();
      releaseReplaced();
      stream?.close();
    },
  };
}

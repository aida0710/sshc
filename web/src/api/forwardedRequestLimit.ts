// A browser opens at most six HTTP/1.1 connections to the engine, and every request of this page shares them. A
// request forwarded to a run target keeps one until the host answers, which takes as long as the SSH connection
// takes to fail when the host is silent. Left alone, a silent host stops this machine's own requests too, the SSH
// config and the settings among them, so half of the connections are kept for those.
const maximumForwardedRequests = 3;

const forwardedRequestPrefix = "/api/v1/engines/";

let running = 0;
const waiting: (() => void)[] = [];

// isForwardedToRunTarget says whether a request goes over SSH. The list of run targets (/api/v1/engines) does not.
export function isForwardedToRunTarget(path: string): boolean {
  return path.startsWith(forwardedRequestPrefix);
}

// withinForwardedRequestLimit sends a forwarded request once a connection is free for it. A request aborted while it
// waits is never sent. The connection counts as free once the answer's headers arrive: forwarded answers are short.
export async function withinForwardedRequestLimit<Answer>(
  signal: AbortSignal | null | undefined,
  send: () => Promise<Answer>,
): Promise<Answer> {
  await takeConnection(signal);
  try {
    return await send();
  } finally {
    running -= 1;
    waiting.shift()?.();
  }
}

function takeConnection(signal: AbortSignal | null | undefined): Promise<void> {
  signal?.throwIfAborted();
  if (running < maximumForwardedRequests) {
    running += 1;
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const start = () => {
      signal?.removeEventListener("abort", giveUp);
      running += 1;
      resolve();
    };
    const giveUp = () => {
      waiting.splice(waiting.indexOf(start), 1);
      reject(signal?.reason as Error);
    };
    signal?.addEventListener("abort", giveUp, { once: true });
    waiting.push(start);
  });
}

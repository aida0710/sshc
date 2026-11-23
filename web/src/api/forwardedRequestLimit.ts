// A browser opens at most six HTTP/1.1 connections to the engine, and every request of this page shares them. A
// request forwarded to a run target keeps one until the host answers, which takes as long as the SSH connection
// takes to fail when the host is silent. Left alone, a silent host stops this machine's own requests too, the SSH
// config and the settings among them, so half of the connections are kept for those.
const maximumForwardedRequests = 3;
// A request that carries a file (an upload's part, a download, a preview) holds its connection for as long as the
// file takes, and a large file goes in several parts at once. They may hold all but one of the forwarded
// connections: the last is kept for the short requests, so the list of sessions, the progress of the transfers and
// opening a session still answer while files move.
const maximumForwardedFileRequests = maximumForwardedRequests - 1;

const forwardedRequestPrefix = "/api/v1/engines/";

let running = 0;
let runningFiles = 0;
// start takes the connection if one is free for the waiting request, and says whether it did.
const waiting: (() => boolean)[] = [];

// carriesFile says whether a forwarded request carries a file's contents: the routes this machine's engine forwards
// as streams (forwardedRoutes with `file` in internal/httpserver/remote_engine_forwards.go).
export function carriesFile(method: string, path: string): boolean {
  const route = new URL(path, "http://engine.invalid").pathname;
  if (!/^\/api\/v1\/engines\/[^/]+\/sftp\//.test(route)) return false;
  if (/\/uploads\/[^/]+$/.test(route)) return method.toUpperCase() === "PATCH";
  return /\/(download|archive|preview|thumbnail|text)$/.test(route);
}

// isForwardedToRunTarget says whether a request goes over SSH. The list of run targets (/api/v1/engines) does not.
export function isForwardedToRunTarget(path: string): boolean {
  return path.startsWith(forwardedRequestPrefix);
}

// withinForwardedRequestLimit sends a forwarded request once a connection is free for it. A request aborted while it
// waits is never sent. The connection counts as free once the answer's headers arrive. `file` says that the request
// carries a file (carriesFile).
export async function withinForwardedRequestLimit<Answer>(
  signal: AbortSignal | null | undefined,
  send: () => Promise<Answer>,
  file = false,
): Promise<Answer> {
  await takeConnection(signal, file);
  try {
    return await send();
  } finally {
    running -= 1;
    if (file) runningFiles -= 1;
    startWaiting();
  }
}

function connectionFree(file: boolean): boolean {
  return running < maximumForwardedRequests && (!file || runningFiles < maximumForwardedFileRequests);
}

// startWaiting hands the freed connections to the requests that waited longest and can use them: a short request
// goes ahead of a file that still has to wait for its own share.
function startWaiting() {
  for (let index = 0; index < waiting.length && running < maximumForwardedRequests;) {
    if (waiting[index]!()) waiting.splice(index, 1);
    else index += 1;
  }
}

function takeConnection(signal: AbortSignal | null | undefined, file: boolean): Promise<void> {
  signal?.throwIfAborted();
  const take = () => {
    running += 1;
    if (file) runningFiles += 1;
  };
  if (connectionFree(file)) {
    take();
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const start = () => {
      if (!connectionFree(file)) return false;
      signal?.removeEventListener("abort", giveUp);
      take();
      resolve();
      return true;
    };
    const giveUp = () => {
      waiting.splice(waiting.indexOf(start), 1);
      reject(signal?.reason as Error);
    };
    signal?.addEventListener("abort", giveUp, { once: true });
    waiting.push(start);
  });
}

import { sftpApi } from "./api";
import { blobDataURL } from "./blobDataURL";

// maxThumbnailsInFlight is how many thumbnails the page asks for at once. The engine makes a host's one at a time, so
// a longer line would only be requests that a scroll has to cancel. The rest of the page's requests keep the
// browser's other connections.
const maxThumbnailsInFlight = 2;

export type ThumbnailRequest = { alias: string; path: string; revision: string };
// finished gets the picture, or null when the file has none or it could not be made now: the row keeps its icon.
type ThumbnailJob = { request: ThumbnailRequest; finished: (url: string | null) => void; controller: AbortController };

const waiting: ThumbnailJob[] = [];
let inFlight = 0;

async function run(job: ThumbnailJob): Promise<void> {
  const { request, controller, finished } = job;
  try {
    const url = await blobDataURL(await sftpApi.thumbnail(request, controller.signal));
    if (!controller.signal.aborted) finished(url);
  } catch {
    if (!controller.signal.aborted) finished(null);
  }
}

function startNext() {
  while (inFlight < maxThumbnailsInFlight) {
    const job = waiting.shift();
    if (job === undefined) return;
    inFlight += 1;
    void run(job).finally(() => {
      inFlight -= 1;
      startNext();
    });
  }
}

// requestThumbnail lines up one thumbnail and returns what cancels it: taken out of the line if it is still waiting,
// and stopped if the engine is making it, which stops reading the file there within a chunk.
export function requestThumbnail(request: ThumbnailRequest, finished: (url: string | null) => void): () => void {
  const job: ThumbnailJob = { request, finished, controller: new AbortController() };
  waiting.push(job);
  startNext();
  return () => {
    job.controller.abort();
    const position = waiting.indexOf(job);
    if (position >= 0) waiting.splice(position, 1);
  };
}

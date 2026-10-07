import { uiCacheName, uiCacheCompletionURL, releaseBaseURL } from "./ui-cache-addresses.js";
import { populateArchiveCache } from "./archive-cache.js";
import { connectCacheWorker } from "./cache-worker-connection.js";

export async function prepareUIArchive({ archive, baseURL, onProgress }) {
  if (!navigator.serviceWorker || !globalThis.caches || !globalThis.DecompressionStream) {
    throw new Error("Browser archive support is unavailable");
  }
  const cache = await caches.open(uiCacheName(baseURL));
  const completed = await cache.match(uiCacheCompletionURL(baseURL));
  const hasCompleteCache = completed && await completed.text() === archive.sha256 &&
    (await cache.keys()).length === archive.fileCount + 1;
  if (hasCompleteCache) onProgress({ state: "ready", fraction: 1 });
  else {
    try {
      await populateArchiveCache({ archive, archiveURL: new URL(`../${archive.fileName}`, baseURL), baseURL, cache, onProgress });
    } catch (error) {
      await caches.delete(uiCacheName(baseURL));
      throw error;
    }
  }
  if (releaseBaseURL(baseURL)) return;
  await connectCacheWorker(new URL("../ui-cache-worker.js", baseURL));
}

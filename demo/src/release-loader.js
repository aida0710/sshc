import { populateArchiveCache } from "./archive-cache.js";
import { connectCacheWorker } from "./cache-worker-connection.js";
import { releaseBaseURL, releaseCacheName, uiCacheCompletionURL } from "./ui-cache-addresses.js";
import { validateReleaseManifest } from "./release-manifest.js";

// A one-use ticket preserves the user's click across the navigation into the selected bundle.
const consentStorageKey = "sshc-demo-release-consent";
// The small preflight lookup must not prevent the installed snapshot from starting indefinitely.
const releaseLookupTimeoutMs = 10000;

export function consumeReleaseConsent(version) {
  const consent = sessionStorage.getItem(consentStorageKey);
  sessionStorage.removeItem(consentStorageKey);
  return !!releaseBaseURL(new URL(location.href)) && consent === version;
}

export async function fetchLatestRelease(proxyURL) {
  if (!proxyURL) return null;
  const url = new URL(proxyURL);
  const response = await fetch(new URL("latest.json", url), { cache: "no-store",
    signal: AbortSignal.timeout(releaseLookupTimeoutMs) });
  if (!response.ok) throw new Error("Latest release is unavailable");
  return validateReleaseManifest(await response.json(), url);
}

export async function openRelease({ manifest, onProgress }) {
  const baseURL = new URL(`/github-releases/${manifest.version}/`, location.origin);
  const cacheName = releaseCacheName(baseURL);
  const cache = await caches.open(cacheName);
  const completed = await cache.match(uiCacheCompletionURL(baseURL));
  const hasCompleteCache = completed && await completed.text() === manifest.sha256 &&
    (await cache.keys()).length === manifest.fileCount + 1;
  if (!hasCompleteCache) {
    try {
      await populateArchiveCache({ archive: manifest, archiveURL: manifest.archiveURL, baseURL, cache, onProgress });
      const configuration = await (await cache.match(new URL("config.json", baseURL)))?.json();
      if (configuration?.version !== manifest.version || configuration.imageBaseURL !== "./images/") {
        throw new Error("Release configuration does not match its manifest");
      }
    } catch (error) {
      await caches.delete(cacheName);
      throw error;
    }
  }
  await connectCacheWorker(new URL("/demo-release-worker.js", location.origin), { requireController: false });
  sessionStorage.setItem(consentStorageKey, manifest.version);
  location.assign(new URL("index.html", baseURL));
}

export async function deleteReleaseCache() {
  const baseURL = releaseBaseURL(new URL(location.href));
  if (baseURL) await caches.delete(releaseCacheName(baseURL));
}

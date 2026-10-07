import { readTarFiles } from "./tar-archive.js";
import { uiCacheName, uiCacheCompletionURL } from "./ui-cache-addresses.js";

const contentTypes = {
  html: "text/html; charset=utf-8", js: "text/javascript; charset=utf-8", css: "text/css; charset=utf-8",
  json: "application/json", webmanifest: "application/manifest+json", svg: "image/svg+xml",
  png: "image/png", woff2: "font/woff2", ttf: "font/ttf", txt: "text/plain; charset=utf-8",
  wasm: "application/wasm",
};

export async function prepareUIArchive({ archive, baseURL, onProgress }) {
  if (!navigator.serviceWorker || !globalThis.caches || !globalThis.DecompressionStream) {
    throw new Error("Browser archive support is unavailable");
  }
  const cache = await caches.open(uiCacheName(baseURL));
  const completed = await cache.match(uiCacheCompletionURL(baseURL));
  const hasCompleteCache = completed && await completed.text() === archive.sha256 &&
    (await cache.keys()).length === archive.fileCount + 1;
  if (hasCompleteCache) onProgress({ state: "ready", fraction: 1 });
  else await populateUICache({ archive, baseURL, cache, onProgress });
  await connectCacheWorker(new URL("../ui-cache-worker.js", baseURL));
}

async function populateUICache({ archive, baseURL, cache, onProgress }) {
  try {
    const compressed = await downloadArchive(new URL(`../${archive.fileName}`, baseURL), archive.bytes, onProgress);
    const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", compressed));
    const sha256 = Array.from(digest, (byte) => byte.toString(16).padStart(2, "0")).join("");
    if (sha256 !== archive.sha256) throw new Error("UI archive checksum mismatch");
    onProgress({ state: "unpacking", fraction: 1 });
    const stream = new Blob([compressed]).stream().pipeThrough(new DecompressionStream("gzip"));
    const files = readTarFiles(new Uint8Array(await new Response(stream).arrayBuffer()));
    if (files.length !== archive.fileCount || !files.some((file) => file.path === "index.html")) {
      throw new Error("Incomplete UI archive");
    }
    // No completion marker is written until every file is stored successfully.
    for (const request of await cache.keys()) await cache.delete(request);
    for (const [index, file] of files.entries()) {
      const extension = file.path.split(".").at(-1);
      await cache.put(new URL(file.path, baseURL), new Response(file.bytes, {
        headers: { "Content-Type": contentTypes[extension] ?? "application/octet-stream" },
      }));
      onProgress({ state: "caching", fraction: 1, completedFiles: index + 1, totalFiles: files.length });
    }
    await cache.put(uiCacheCompletionURL(baseURL), new Response(archive.sha256));
    onProgress({ state: "ready", fraction: 1 });
  } catch (error) {
    await caches.delete(uiCacheName(baseURL));
    throw error;
  }
}

async function downloadArchive(url, expectedBytes, onProgress) {
  const response = await fetch(url);
  if (!response.ok) throw new Error("UI archive is unavailable");
  const reader = response.body.getReader();
  const chunks = [];
  let loaded = 0;
  onProgress({ state: "downloading", fraction: 0 });
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    chunks.push(value);
    loaded += value.length;
    onProgress({ state: "downloading", fraction: Math.min(1, loaded / expectedBytes) });
  }
  return new Blob(chunks).arrayBuffer();
}

async function connectCacheWorker(workerURL) {
  const registration = await navigator.serviceWorker.register(workerURL, { type: "module" });
  if (!registration.active || registration.active.state !== "activated") {
    const worker = registration.installing ?? registration.waiting ?? registration.active;
    if (!worker) throw new Error("UI cache worker is unavailable");
    await new Promise((resolve, reject) => {
      const checkState = () => {
        if (worker.state === "activated") resolve();
        else if (worker.state === "redundant") reject(new Error("UI cache worker failed"));
      };
      worker.addEventListener("statechange", checkState);
      checkState();
    });
  }
  if (navigator.serviceWorker.controller?.scriptURL === workerURL.href) return;
  await new Promise((resolve) => {
    const checkController = () => {
      if (navigator.serviceWorker.controller?.scriptURL !== workerURL.href) return;
      navigator.serviceWorker.removeEventListener("controllerchange", checkController);
      resolve();
    };
    navigator.serviceWorker.addEventListener("controllerchange", checkController);
    checkController();
  });
}

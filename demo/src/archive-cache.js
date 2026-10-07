import { readTarFiles } from "./tar-archive.js";
import { uiCacheCompletionURL } from "./ui-cache-addresses.js";

const contentTypes = {
  html: "text/html; charset=utf-8", js: "text/javascript; charset=utf-8", css: "text/css; charset=utf-8",
  json: "application/json", webmanifest: "application/manifest+json", svg: "image/svg+xml",
  png: "image/png", woff2: "font/woff2", ttf: "font/ttf", txt: "text/plain; charset=utf-8",
  wasm: "application/wasm",
};

export async function populateArchiveCache({ archive, archiveURL, baseURL, cache, onProgress }) {
  const compressed = await downloadArchive(archiveURL, archive.bytes, onProgress);
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", compressed));
  const sha256 = Array.from(digest, (byte) => byte.toString(16).padStart(2, "0")).join("");
  if (sha256 !== archive.sha256) throw new Error("Demo archive checksum mismatch");
  onProgress({ state: "unpacking", fraction: 1 });
  const stream = new Blob([compressed]).stream().pipeThrough(new DecompressionStream("gzip"));
  const files = readTarFiles(new Uint8Array(await new Response(stream).arrayBuffer()));
  if (files.length !== archive.fileCount || !files.some((file) => file.path === "index.html")) {
    throw new Error("Incomplete demo archive");
  }
  for (const request of await cache.keys()) await cache.delete(request);
  for (const [index, file] of files.entries()) {
    const extension = file.path.split(".").at(-1);
    await cache.put(new URL(file.path, baseURL), new Response(file.bytes, {
      headers: { "Content-Type": contentTypes[extension] ?? "application/octet-stream" },
    }));
    onProgress({ state: "caching", fraction: 1, completedFiles: index + 1, totalFiles: files.length });
  }
  // The completion marker is committed only after every file has arrived.
  await cache.put(uiCacheCompletionURL(baseURL), new Response(archive.sha256));
  onProgress({ state: "ready", fraction: 1 });
}

async function downloadArchive(url, expectedBytes, onProgress) {
  const response = await fetch(url);
  if (!response.ok) throw new Error("Demo archive is unavailable");
  const reader = response.body.getReader();
  const chunks = [];
  let loaded = 0;
  onProgress({ state: "downloading", fraction: 0 });
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    loaded += value.length;
    if (loaded > expectedBytes) {
      await reader.cancel();
      throw new Error("Demo archive exceeds its declared size");
    }
    chunks.push(value);
    onProgress({ state: "downloading", fraction: Math.min(1, loaded / expectedBytes) });
  }
  if (loaded !== expectedBytes) throw new Error("Demo archive size mismatch");
  return new Blob(chunks).arrayBuffer();
}


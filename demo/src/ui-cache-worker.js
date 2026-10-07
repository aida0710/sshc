import { uiCacheName } from "./ui-cache-addresses.js";

const baseURL = new URL("./ui/", self.location.href);
self.addEventListener("install", (event) => event.waitUntil(self.skipWaiting()));
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== "GET" || url.origin !== baseURL.origin || !url.pathname.startsWith(baseURL.pathname)) return;
  // UI files exist only in the archive. API/VM requests never enter this cache.
  event.respondWith(caches.open(uiCacheName(baseURL)).then(async (cache) =>
    await cache.match(event.request, { ignoreSearch: true }) ?? new Response("Missing UI asset", { status: 404 })));
});

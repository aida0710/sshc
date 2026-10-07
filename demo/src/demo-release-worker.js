import { releaseBaseURL, releaseCacheName, uiCacheName } from "./ui-cache-addresses.js";

self.addEventListener("install", (event) => event.waitUntil(self.skipWaiting()));
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  const baseURL = releaseBaseURL(url);
  if (event.request.method !== "GET" || url.origin !== self.location.origin || !baseURL) return;
  const uiBaseURL = new URL("ui/", baseURL);
  const cacheName = url.pathname.startsWith(uiBaseURL.pathname) ? uiCacheName(uiBaseURL) : releaseCacheName(baseURL);
  event.respondWith(caches.match(event.request, { cacheName, ignoreSearch: true }).then((response) =>
    response ?? new Response("Missing demo asset", { status: 404 })));
});

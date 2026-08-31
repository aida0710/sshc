const cacheName = "sshc-offline-v3";
const offlineAssets = [
  "/offline.html",
  "/offline.js",
  "/logo.svg",
  "/icon-192.png",
  "/icon-512.png",
  "/icon-maskable-512.png?v=2",
  "/manifest.webmanifest",
];
const offlineAssetPaths = new Set(offlineAssets.map((asset) => new URL(asset, self.location.origin).pathname));

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(cacheName).then((cache) => cache.addAll(offlineAssets)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((names) => Promise.all(names.filter((name) => name !== cacheName).map((name) => caches.delete(name)))),
  );
  self.clients.claim();
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.mode === "navigate") {
    event.respondWith(fetch(request).catch(() => caches.match("/offline.html")));
    return;
  }
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || !offlineAssetPaths.has(url.pathname)) return;
  event.respondWith(caches.match(request).then((cached) => cached ?? fetch(request)));
});

/* This script belongs only to the demo's iframe; the native UI keeps its normal transport. */
(() => {
  // The native router expects /, /connections, etc. Keep asset resolution at the
  // static UI directory while giving this iframe the native route paths.
  const assetBase = document.createElement("base");
  assetBase.href = new URL("./", location.href).href;
  document.head.append(assetBase);
  history.replaceState(null, "", "/" + location.search + location.hash);
  const channel = "sshc-demo";
  // Keep base64 requests below the guest's serial line limit.
  const maxBodyBytes = 2 * 1024 * 1024;
  const pendingRequests = new Map();
  const sockets = new Map();
  const originalFetch = window.fetch.bind(window);
  let nextRequestID = 0;
  let hasNotifiedUIReady = false;
  const send = (request) => parent.postMessage({ channel, request }, location.origin);
  const nextID = () => String(++nextRequestID);
  const encodeBytes = (bytes) => {
    let encoded = "";
    for (const byte of bytes) encoded += String.fromCharCode(byte);
    return btoa(encoded);
  };
  const decodeBytes = (body) => Uint8Array.from(atob(body ?? ""), (character) => character.charCodeAt(0));

  // The product reads its language from this key (web/src/ui/browserStorageKeys.ts); the demo
  // page has already chosen one, so the Web UI opens in the same language.
  const productLanguageKey = "sshc.language";
  let demoLanguage = "";
  try {
    demoLanguage = parent.document.documentElement.lang;
  } catch {
    // Only the demo page may host this UI; a foreign parent keeps the browser language.
  }
  // A new VM starts a new demo; tokens and settings from previous VMs must not survive.
  for (const storageName of ["localStorage", "sessionStorage"]) {
    const entries = new Map(storageName === "localStorage" && demoLanguage ? [[productLanguageKey, demoLanguage]] : []);
    Object.defineProperty(window, storageName, { value: {
      getItem: (name) => entries.get(name) ?? null,
      setItem: (name, value) => entries.set(name, String(value)),
      removeItem: (name) => entries.delete(name),
      clear: () => entries.clear(),
      key: (index) => [...entries.keys()][index] ?? null,
      get length() { return entries.size; },
    } });
  }
  // Only the archive worker may serve this demo; disable the product's PWA worker.
  if (navigator.serviceWorker) {
    navigator.serviceWorker.register = async () => { throw new Error("Demo uses static assets"); };
  }

  window.fetch = async (input, options = {}) => {
    const request = new Request(input, options);
    const url = new URL(request.url);
    if (url.origin !== location.origin || !url.pathname.startsWith("/api/v1/")) {
      return originalFetch(input, options);
    }
    const id = nextID();
    const bytes = request.body ? new Uint8Array(await request.arrayBuffer()) : new Uint8Array();
    if (bytes.length > maxBodyBytes) throw new Error("Demo requests are limited to 2 MiB");
    const body = encodeBytes(bytes);
    return new Promise((resolve, reject) => {
      pendingRequests.set(id, { resolve, reject, path: url.pathname });
      send({ id, kind: "fetch", path: url.pathname + url.search, method: request.method,
        headers: Object.fromEntries(request.headers), body });
    });
  };

  class DemoWebSocket extends EventTarget {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
    CONNECTING = 0;
    OPEN = 1;
    CLOSING = 2;
    CLOSED = 3;
    readyState = 0;
    binaryType = "blob";
    bufferedAmount = 0;

    constructor(address) {
      super();
      this.id = nextID();
      this.url = String(address);
      const url = new URL(address);
      if (url.host !== location.host || url.pathname !== "/terminal/stream") {
        throw new Error("Only demo terminal streams are available");
      }
      sockets.set(this.id, this);
      send({ id: this.id, kind: "open", path: url.pathname + url.search });
    }

    send(body) {
      if (this.readyState !== DemoWebSocket.OPEN) throw new Error("WebSocket is not open");
      if (typeof body === "string") {
        send({ id: this.id, kind: "send", text: body });
        return;
      }
      const bytes = body instanceof ArrayBuffer ? new Uint8Array(body) : body;
      send({ id: this.id, kind: "send", body: encodeBytes(bytes) });
    }

    close() {
      if (this.readyState === DemoWebSocket.CLOSED) return;
      this.readyState = DemoWebSocket.CLOSING;
      send({ id: this.id, kind: "close" });
    }
  }
  window.WebSocket = DemoWebSocket;

  window.addEventListener("message", (event) => {
    if (event.origin !== location.origin || event.source !== parent || event.data?.channel !== channel) return;
    const reply = event.data.reply;
    const pending = pendingRequests.get(reply.id);
    if (pending) {
      pendingRequests.delete(reply.id);
      if (reply.kind === "error") {
        pending.reject(new Error(reply.text));
        if (!hasNotifiedUIReady) parent.postMessage({ channel, startup: "ui-failed" }, location.origin);
      }
      else {
        const headers = new Headers();
        for (const [name, values] of Object.entries(reply.headers ?? {})) {
          if (name.toLowerCase() === "set-cookie") continue;
          for (const value of values) headers.append(name, value);
        }
        const body = [204, 205, 304].includes(reply.status) ? null : decodeBytes(reply.body);
        pending.resolve(new Response(body, { status: reply.status, headers }));
        // The product enters its ready state after bootstrap, health and Vault status.
        if (!hasNotifiedUIReady && pending.path === "/api/v1/passwords" && reply.status === 200) {
          try {
            if (JSON.parse(new TextDecoder().decode(body)).unlocked === true) {
              hasNotifiedUIReady = true;
              parent.postMessage({ channel, startup: "ui-ready" }, location.origin);
            }
          } catch {
            parent.postMessage({ channel, startup: "ui-failed" }, location.origin);
          }
        }
        if (!hasNotifiedUIReady && reply.status >= 400 &&
            ["/api/v1/session/bootstrap", "/api/v1/health", "/api/v1/passwords"].includes(pending.path)) {
          parent.postMessage({ channel, startup: "ui-failed" }, location.origin);
        }
      }
      return;
    }
    const socket = sockets.get(reply.id);
    if (!socket) return;
    if (reply.kind === "opened") {
      socket.readyState = DemoWebSocket.OPEN;
      socket.dispatchEvent(new Event("open"));
    } else if (reply.kind === "message") {
      const bytes = decodeBytes(reply.body);
      socket.dispatchEvent(new MessageEvent("message", {
        data: reply.text ?? (socket.binaryType === "arraybuffer" ? bytes.buffer : new Blob([bytes])),
      }));
    } else if (reply.kind === "closed" || reply.kind === "error") {
      socket.readyState = DemoWebSocket.CLOSED;
      sockets.delete(reply.id);
      if (reply.kind === "error") socket.dispatchEvent(new Event("error"));
      socket.dispatchEvent(new CloseEvent("close"));
    }
  });
})();

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const publicDirectory = join(dirname(fileURLToPath(import.meta.url)), "..", "public");

describe("offline fallback", () => {
  it("uses a cached same-origin script for reload under the production CSP", () => {
    const html = readFileSync(join(publicDirectory, "offline.html"), "utf8");
    const script = readFileSync(join(publicDirectory, "offline.js"), "utf8");
    const serviceWorker = readFileSync(join(publicDirectory, "sw.js"), "utf8");

    expect(html).toContain('<button id="offline-reload" type="button">');
    expect(html).toContain('<script src="/offline.js"></script>');
    expect(html).not.toMatch(/\son\w+\s*=/i);
    expect(script).toContain('addEventListener("click"');
    expect(script).toContain("window.location.reload()");
    expect(serviceWorker).toContain('"/offline.js"');
    expect(serviceWorker).toContain("offlineAssets.includes(url.pathname)");
    expect(serviceWorker).toContain("caches.match(request)");
  });
});

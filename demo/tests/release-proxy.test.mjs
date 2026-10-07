import assert from "node:assert/strict";
import test from "node:test";
import { proxyReleaseRequest } from "../release-proxy/worker.js";
import { validateReleaseManifest } from "../src/release-manifest.js";

const version = "v0.44.0";
const manifest = { format: 1, version, fileName: `sshc-demo-${version}.tar.gz`, bytes: 123,
  fileCount: 40, sha256: "a".repeat(64) };
const proxyURL = new URL("https://demo-releases.example/");

test("最新タグのredirectとmanifestから同じ版のダウンロード先を返す", async () => {
  const requests = [];
  const response = await proxyReleaseRequest(new Request(new URL("latest.json", proxyURL)), {
    fetchUpstream: async (url, options) => {
      requests.push(url);
      if (url.endsWith("/latest")) {
        assert.equal(options.redirect, "manual");
        return new Response(null, { status: 302, headers: { Location: `https://github.com/aida0710/sshc/releases/tag/${version}` } });
      }
      return Response.json(manifest);
    },
  });
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("Access-Control-Allow-Origin"), "*");
  const selected = validateReleaseManifest(await response.json(), proxyURL);
  assert.equal(selected.version, version);
  assert.equal(selected.archiveURL.href, `https://demo-releases.example/archives/${version}.tar.gz`);
  assert.deepEqual(requests, ["https://github.com/aida0710/sshc/releases/latest",
    `https://github.com/aida0710/sshc/releases/download/${version}/sshc-demo-${version}.json`]);
});

test("GitHub以外へのredirectを受け入れず外部サイトを取得しない", async () => {
  let calls = 0;
  const response = await proxyReleaseRequest(new Request(new URL("latest.json", proxyURL)), {
    fetchUpstream: async () => {
      calls++;
      return new Response(null, { status: 302, headers: { Location: `https://other.example/aida0710/sshc/releases/tag/${version}` } });
    },
  });
  assert.equal(response.status, 502);
  assert.equal(calls, 1);
});

test("デモを含まない旧Releaseは未対応として返す", async () => {
  const response = await proxyReleaseRequest(new Request(new URL("latest.json", proxyURL)), {
    fetchUpstream: async (url) => url.endsWith("/latest")
      ? new Response(null, { status: 302, headers: { Location: `https://github.com/aida0710/sshc/releases/tag/${version}` } })
      : new Response(null, { status: 404 }),
  });
  assert.equal(response.status, 404);
  assert.equal(response.headers.get("Cache-Control"), "no-store");
});

test("任意URLと書き込み要求は中継しない", async () => {
  for (const [path, method, expectedStatus] of [["/archives/v0.44.0.tar.gz", "POST", 405], ["/?url=https://other.example", "GET", 404]]) {
    const response = await proxyReleaseRequest(new Request(new URL(path, proxyURL), { method }), {
      fetchUpstream: () => { throw new Error("Must not fetch"); },
    });
    assert.equal(response.status, expectedStatus);
  }
});

test("ブラウザは過大なサイズと別オリジンのアーカイブを拒否する", () => {
  const valid = { ...manifest, archiveURL: new URL(`/archives/${version}.tar.gz`, proxyURL).href };
  for (const invalid of [{ ...valid, bytes: 129 * 1024 * 1024 }, { ...valid, version: "v0.44.1" },
    { ...valid, archiveURL: `https://other.example/archives/${version}.tar.gz` }]) {
    assert.throws(() => validateReleaseManifest(invalid, proxyURL), /Invalid/);
  }
});

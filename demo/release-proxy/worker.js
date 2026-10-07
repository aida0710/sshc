// Public release files only; this worker never receives an R2 or GitHub write key.
const repository = "aida0710/sshc";
const stableVersion = /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;
// Twelve lookups per hour avoid the anonymous REST API's shared-IP limits.
const latestCacheSeconds = 300;
const archiveCacheSeconds = 31536000;

export default {
  async fetch(request, environment, context) {
    return proxyReleaseRequest(request, { fetchUpstream: fetch, cache: caches.default,
      waitUntil: (promise) => context.waitUntil(promise) });
  },
};

export async function proxyReleaseRequest(request, { fetchUpstream, cache, waitUntil }) {
  if (request.method === "OPTIONS") return new Response(null, { status: 204, headers: corsHeaders() });
  if (request.method !== "GET") return proxyError("Method not allowed", 405);
  const url = new URL(request.url);
  const archiveMatch = /^\/archives\/(v[0-9]+\.[0-9]+\.[0-9]+)\.tar\.gz$/.exec(url.pathname);
  if (url.pathname !== "/latest.json" && !archiveMatch) return proxyError("Not found", 404);
  const cacheKey = new Request(url.origin + url.pathname);
  const cached = await cache?.match(cacheKey);
  if (cached) return cached;
  let response;
  try {
    if (archiveMatch) {
      const version = archiveMatch[1];
      const upstream = await fetchUpstream(releaseAssetURL(version, `sshc-demo-${version}.tar.gz`));
      if (!upstream.ok) return proxyError("Release archive is unavailable", upstream.status === 404 ? 404 : 502);
      response = new Response(upstream.body, { headers: {
        ...corsHeaders(), "Content-Type": "application/octet-stream",
        "Cache-Control": `public, max-age=${archiveCacheSeconds}, immutable`,
      } });
    } else {
      response = await latestManifest(url.origin, fetchUpstream);
    }
  } catch {
    return proxyError("Release lookup failed", 502);
  }
  if (response.ok && cache) waitUntil(cache.put(cacheKey, response.clone()));
  return response;
}

async function latestManifest(origin, fetchUpstream) {
  // GitHub's public latest redirect identifies the version without using its REST quota.
  const latest = await fetchUpstream(`https://github.com/${repository}/releases/latest`, { redirect: "manual" });
  const location = latest.headers.get("Location");
  if (latest.status !== 302 || !location) return proxyError("Latest release is unavailable", 502);
  const target = new URL(location, "https://github.com");
  const prefix = `/${repository}/releases/tag/`;
  const version = target.pathname.slice(prefix.length);
  if (target.origin !== "https://github.com" || !target.pathname.startsWith(prefix) || !stableVersion.test(version)) {
    return proxyError("Invalid latest release", 502);
  }
  const upstream = await fetchUpstream(releaseAssetURL(version, `sshc-demo-${version}.json`));
  if (!upstream.ok) return proxyError("This release has no browser demo", upstream.status === 404 ? 404 : 502);
  const manifest = await upstream.json();
  if (manifest.format !== 1 || manifest.version !== version || manifest.fileName !== `sshc-demo-${version}.tar.gz` ||
      !/^[a-f0-9]{64}$/.test(manifest.sha256) || !Number.isSafeInteger(manifest.bytes) || manifest.bytes <= 0 ||
      !Number.isSafeInteger(manifest.fileCount) || manifest.fileCount <= 0) {
    return proxyError("Invalid release manifest", 502);
  }
  return Response.json({ ...manifest, archiveURL: `${origin}/archives/${version}.tar.gz` }, { headers: {
    ...corsHeaders(), "Cache-Control": `public, max-age=${latestCacheSeconds}`,
  } });
}

function releaseAssetURL(version, name) {
  return `https://github.com/${repository}/releases/download/${version}/${name}`;
}

function corsHeaders() {
  return { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Methods": "GET, OPTIONS" };
}

function proxyError(message, status) {
  return Response.json({ error: message }, { status, headers: { ...corsHeaders(), "Cache-Control": "no-store" } });
}

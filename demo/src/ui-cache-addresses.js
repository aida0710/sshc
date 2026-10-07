export function uiCacheName(baseURL) {
  // Each release keeps its own cache so a newer demo cannot break an already open one.
  return `sshc-demo-ui:${baseURL.href}`;
}

export function uiCacheCompletionURL(baseURL) {
  return new URL(".archive-complete", baseURL);
}

export function releaseCacheName(baseURL) {
  return `sshc-demo-release:${baseURL.href}`;
}

export function releaseBaseURL(url) {
  const path = /^\/github-releases\/v[0-9]+\.[0-9]+\.[0-9]+\//.exec(url.pathname)?.[0];
  return path ? new URL(path, url.origin) : null;
}

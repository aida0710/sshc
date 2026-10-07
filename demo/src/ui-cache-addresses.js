export function uiCacheName(baseURL) {
  // Each release keeps its own cache so a newer demo cannot break an already open one.
  return `sshc-demo-ui:${baseURL.href}`;
}

export function uiCacheCompletionURL(baseURL) {
  return new URL(".archive-complete", baseURL);
}

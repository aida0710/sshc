// ':' and '/' are rejected in SSH aliases, so the engine filesystem has an
// unambiguous identity in the same tab model as remote connections.
export const localHostAlias = "sshc://local";

export function isLocalPath(path: string): boolean {
  return path.startsWith("/") || /^[A-Za-z]:\//.test(path);
}

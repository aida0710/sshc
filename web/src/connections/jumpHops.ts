import type { HostEntry } from "../api/config";

// jumpHopNames returns the connection names a ProxyJump value goes through, in order: the host of each
// [user@]host[:port] hop, or of an ssh:// URI, as the engine reads them when it renames a connection
// (internal/application/jump_rename.go). An IPv6 address in brackets and "none" name no connection.
export function jumpHopNames(value: string): string[] {
  const names: string[] = [];
  for (const hop of value.split(",")) {
    let rest = hop.trim();
    if (rest.toLowerCase().startsWith("ssh://")) rest = rest.slice("ssh://".length);
    const at = rest.lastIndexOf("@");
    if (at >= 0) rest = rest.slice(at + 1);
    if (rest === "" || rest.startsWith("[")) continue;
    const colon = rest.lastIndexOf(":");
    const host = colon >= 0 ? rest.slice(0, colon) : rest;
    if (host !== "" && host.toLowerCase() !== "none") names.push(host);
  }
  return names;
}

// ssh compares a hop with the Host patterns in lower case, so a connection named in another case is the same one.
function sameName(left: string, right: string): boolean {
  return left.toLowerCase() === right.toLowerCase();
}

// connectionsJumpingThrough returns the connections, other than those in aliases, whose ProxyJump goes through one of
// aliases: they can no longer connect once those are deleted.
export function connectionsJumpingThrough(hosts: readonly HostEntry[], aliases: readonly string[]): HostEntry[] {
  return hosts.filter((host) =>
    !aliases.some((alias) => sameName(alias, host.identity.alias)) &&
    jumpHopNames(host.proxyJump ?? "").some((hop) => aliases.some((alias) => sameName(alias, hop))));
}

// unsavedJumpHops returns the hops of a ProxyJump value that look like a connection name but name no saved
// connection. ssh takes such a hop for a host name, which is wrong for a connection deleted or renamed elsewhere. A hop
// with a dot (a DNS name or an IPv4 address) is meant as a host name and is left out.
export function unsavedJumpHops(value: string, savedAliases: readonly string[]): string[] {
  return jumpHopNames(value).filter((hop) => !hop.includes(".") && !savedAliases.some((alias) => sameName(alias, hop)));
}

import type { HostIdentity, Overview } from "../api/config";

// Where a host lands after a move is decided by the engine; the page only
// knows the alias and the group it asked for. These helpers find the moved
// host in the refreshed overview so the selection can follow it.

// movedHostIdentity returns the identity the host has after moving into
// group, or undefined when the refreshed overview does not show it there.
export function movedHostIdentity(overview: Overview | null, alias: string, group: string): HostIdentity | undefined {
  return overview?.hosts.find((host) => host.identity.alias === alias && host.group === group)?.identity;
}

// groupAfterRename returns the group the selected host belongs to after the
// group `from` (and everything below it) is renamed to `to`, or null when the
// selection is outside the renamed subtree.
export function groupAfterRename(selectedGroup: string | undefined, from: string, to: string): string | null {
  if (selectedGroup === from) return to;
  if (selectedGroup?.startsWith(`${from}/`)) return `${to}${selectedGroup.slice(from.length)}`;
  return null;
}

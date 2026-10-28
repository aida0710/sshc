// A group name is a path of folders under ~/.ssh/connections ("work/eu"). These are the screen's copies of
// internal/application/grouppath.go and keep its names, so a rule changes in both places at once.

// groupSegments splits a group name into its folders. No group has none.
export function groupSegments(name: string): string[] {
  return name === "" ? [] : name.split("/");
}

// groupDepth counts a group's folders: 0 for no group, 2 for "work/eu".
export function groupDepth(name: string): number {
  return groupSegments(name).length;
}

// parentGroupName is the group a group sits in, or "" at the top.
export function parentGroupName(name: string): string {
  const index = name.lastIndexOf("/");
  return index < 0 ? "" : name.slice(0, index);
}

// groupBaseName is a group's own folder, the last part of its name.
export function groupBaseName(name: string): string {
  return name.slice(name.lastIndexOf("/") + 1);
}

// isGroupOrDescendant reports whether group is ancestor itself or a group inside it, at any depth.
export function isGroupOrDescendant(group: string, ancestor: string): boolean {
  return group === ancestor || group.startsWith(`${ancestor}/`);
}

// groupAncestry is a group and the groups it sits in, nearest first: "work/eu", then "work".
export function groupAncestry(name: string): string[] {
  const names: string[] = [];
  for (let current = name; current !== ""; current = parentGroupName(current)) names.push(current);
  return names;
}

// renamedGroup is where group ends up when the group from, and everything inside it, is renamed to to, or
// null when group is outside it.
export function renamedGroup(group: string, from: string, to: string): string | null {
  return isGroupOrDescendant(group, from) ? `${to}${group.slice(from.length)}` : null;
}

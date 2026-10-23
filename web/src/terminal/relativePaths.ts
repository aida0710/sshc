// resolveRelativePath joins a path a program printed to the directory it works
// in, folding "." and ".." the way the program's own file system would.
export function resolveRelativePath(directory: string, relative: string): string {
  const drive = /^[A-Za-z]:/u.exec(directory)?.[0] ?? "";
  const components: string[] = [];
  for (const component of `${directory.slice(drive.length)}/${relative}`.split("/")) {
    if (component === "" || component === ".") continue;
    if (component === "..") components.pop();
    else components.push(component);
  }
  return `${drive}/${components.join("/")}`;
}

export function isHomePath(path: string): boolean {
  return path === "~" || path.startsWith("~/");
}

// Home paths must never fall back to the working directory: the two can
// belong to entirely different trees, even within the same session.
export function resolveTerminalPath(path: string, directory: string | null, home: string | null): string | null {
  if (path.startsWith("/") || /^[A-Za-z]:\//u.test(path)) return path;
  if (isHomePath(path)) return home === null ? null : resolveRelativePath(home, path.slice(2));
  if (path.startsWith("~")) return null;
  return directory === null ? null : resolveRelativePath(directory, path);
}

// resolveRelativePath joins a path a program printed to the directory it works
// in, folding "." and ".." the way the program's own file system would.
export function resolveRelativePath(directory: string, relative: string): string {
  const components: string[] = [];
  for (const component of `${directory}/${relative}`.split("/")) {
    if (component === "" || component === ".") continue;
    if (component === "..") components.pop();
    else components.push(component);
  }
  return `/${components.join("/")}`;
}

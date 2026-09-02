type Disposable = { dispose(): void };
type OSCParser = { registerOscHandler(identifier: number, handler: (data: string) => boolean): Disposable };

export function parseOSC7Directory(data: string): string | null {
  try {
    const location = new URL(data);
    if (location.protocol !== "file:" || location.username !== "" || location.password !== "" || location.search !== "" || location.hash !== "") return null;
    const path = decodeURIComponent(location.pathname);
    if (!path.startsWith("/") || path.includes("\0") || path.includes("\r") || path.includes("\n")) return null;
    return path;
  } catch {
    return null;
  }
}

export function attachOSC7Directory(parser: OSCParser, changed: (path: string) => void): Disposable {
  return parser.registerOscHandler(7, (data) => {
    const path = parseOSC7Directory(data);
    if (path === null) return false;
    changed(path);
    return true;
  });
}

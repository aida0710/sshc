type Disposable = { dispose(): void };
type OSCParser = { registerOscHandler(identifier: number, handler: (data: string) => boolean): Disposable };

// parseOSC9Directory reads ConEmu's `OSC 9 ; 9 ; "path"` (the text after
// "9;"), which Windows Terminal recommends for PowerShell and cmd prompts. A
// drive path becomes the form OpenSSH for Windows uses over SFTP (C:\Users →
// /C:/Users). Network paths have no SFTP form and are refused.
export function parseOSC9Directory(data: string): string | null {
  if (!data.startsWith("9;")) return null;
  let path = data.slice(2);
  if (path.length >= 2 && path.startsWith("\"") && path.endsWith("\"")) path = path.slice(1, -1);
  if (/[\0\r\n]/.test(path)) return null;
  const drive = /^([A-Za-z]):[\\/]?(.*)$/.exec(path);
  if (drive !== null) {
    const rest = (drive[2] ?? "").replaceAll("\\", "/").replace(/\/+$/, "");
    return `/${(drive[1] ?? "").toUpperCase()}:/${rest}`;
  }
  return path.startsWith("/") && !path.startsWith("//") ? path : null;
}

export function attachOSC9Directory(parser: OSCParser, changed: (path: string) => void): Disposable {
  return parser.registerOscHandler(9, (data) => {
    const path = parseOSC9Directory(data);
    if (path === null) return false;
    changed(path);
    return true;
  });
}

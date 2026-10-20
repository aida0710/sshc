// What a typed remote directory may look like. The engine resolves a path that
// starts at the home directory (~ or ~/…) from the directory the SFTP server
// logs the user into, and answers with the absolute path.

export function startsAtHome(typed: string): boolean {
  return typed === "~" || typed.startsWith("~/");
}

// isRemoteDirectoryInput accepts an absolute path, or one that starts at the home directory.
export function isRemoteDirectoryInput(typed: string): boolean {
  return typed.startsWith("/") || startsAtHome(typed);
}

// The Config Explorer's path field names a place inside ~/.ssh, relative to it
// (conf.d/30-lab.conf). Typed the way a shell writes it (~/.ssh/conf.d), it
// means the same place. Any other path starting at ~ or / is outside ~/.ssh,
// and sending it would make a folder literally named ~ inside ~/.ssh.
const sshDirectoryPrefix = "~/.ssh/";

export type SSHDirectoryPath = { relative: string } | { outside: true };

export function sshDirectoryPath(typed: string): SSHDirectoryPath {
  const trimmed = typed.trim();
  const relative = trimmed.startsWith(sshDirectoryPrefix) ? trimmed.slice(sshDirectoryPrefix.length) : trimmed;
  if (relative === "~" || relative.startsWith("~/") || relative.startsWith("/")) return { outside: true };
  return { relative };
}

export type LocalTransferFile = { file: File; relativePath: string };

export function safeRelativePath(candidate: string): string | null {
  if (candidate.includes("\\")) return null;
  const normalized = candidate.replace(/^\/+/, "");
  const parts = normalized.split("/");
  if (parts.length === 0 || parts.some((part) => part === "" || part === "." || part === ".." || /[\u0000-\u001f]/.test(part))) return null;
  return parts.join("/");
}

export function directoryPaths(files: LocalTransferFile[]): string[] {
  const directories = new Set<string>();
  for (const item of files) {
    const parts = item.relativePath.split("/");
    parts.pop();
    for (let index = 1; index <= parts.length; index += 1) directories.add(parts.slice(0, index).join("/"));
  }
  return [...directories].sort((left, right) => left.split("/").length - right.split("/").length || left.localeCompare(right));
}

export function symbolicModeToOctal(mode: string): string {
  const permissions = mode.slice(-9);
  if (permissions.length !== 9) return "600";
  const digits = [0, 3, 6].map((offset) => {
    let value = 0;
    if (permissions[offset] === "r") value += 4;
    if (permissions[offset + 1] === "w") value += 2;
    if (permissions[offset + 2] !== "-") value += 1;
    return String(value);
  });
  return digits.join("");
}

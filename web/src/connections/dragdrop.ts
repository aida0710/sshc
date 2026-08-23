
export type DragPayload =
  | { kind: "connection"; path: string; alias: string; group: string }
  | { kind: "group"; name: string };

export const dragMimeType = "application/x-sshc-drag";

const maxGroupSegments = 6;

const noGroup = "";

function segments(name: string): number {
  return name === noGroup ? 0 : name.split("/").length;
}

function basename(name: string): string {
  const index = name.lastIndexOf("/");
  return index < 0 ? name : name.slice(index + 1);
}

function isSelfOrDescendant(name: string, candidate: string): boolean {
  return candidate === name || candidate.startsWith(`${name}/`);
}

export function canDrop(payload: DragPayload, target: string, groups: string[]): boolean {
  if (payload.kind === "connection") {
    return payload.group !== target;
  }
  if (isSelfOrDescendant(payload.name, target)) return false;
  const moved = target === noGroup ? basename(payload.name) : `${target}/${basename(payload.name)}`;
  if (moved === payload.name) return false;
  if (groups.includes(moved)) return false;
  return segments(moved) <= maxGroupSegments;
}

import { maxGroupSegments } from "../rules/generated";
import { groupBaseName, groupDepth, isGroupOrDescendant } from "../rules/groupPath";

export type DraggedConnection = { path: string; alias: string; group: string };

export type DragPayload =
  | ({ kind: "connection" } & DraggedConnection)
  // 複数選んだ接続先をまとめて動かす。
  | { kind: "connections"; items: DraggedConnection[] }
  | { kind: "group"; name: string };

export const dragMimeType = "application/x-sshc-drag";

const noGroup = "";


export function canDrop(payload: DragPayload, target: string, groups: string[]): boolean {
  if (payload.kind === "connection") {
    return payload.group !== target;
  }
  if (payload.kind === "connections") {
    return payload.items.some((item) => item.group !== target);
  }
  if (isGroupOrDescendant(target, payload.name)) return false;
  const moved = target === noGroup ? groupBaseName(payload.name) : `${target}/${groupBaseName(payload.name)}`;
  if (moved === payload.name) return false;
  if (groups.includes(moved)) return false;
  return groupDepth(moved) <= maxGroupSegments;
}

export type PortForwardKind = "local" | "remote" | "dynamic";

export const forwardLabelKeys = {
  local: "conn.forwardLocal",
  remote: "conn.forwardRemote",
  dynamic: "conn.forwardDynamic",
} as const;

export const forwardHintKeys = {
  local: "conn.forwardDestinationHint",
  remote: "conn.forwardRemoteHint",
  dynamic: "conn.forwardDynamicHint",
} as const;

export const forwardKeywords = {
  local: "LocalForward",
  remote: "RemoteForward",
  dynamic: "DynamicForward",
} as const;

export function forwardKindForKeyword(keyword: string): PortForwardKind | undefined {
  return (Object.keys(forwardKeywords) as PortForwardKind[]).find((kind) => forwardKeywords[kind].toLowerCase() === keyword.toLowerCase());
}

export function forwardDirective(kind: PortForwardKind, listenPort: string, destination: string) {
  return { keyword: forwardKeywords[kind], values: kind === "dynamic" ? [listenPort] : [listenPort, destination] };
}

export function validForwardPort(value: string): boolean {
  const port = Number(value);
  return /^\d+$/.test(value) && Number.isInteger(port) && port > 0 && port <= 65535;
}

export function validForwardDestination(value: string): boolean {
  if (/[\s\x00-\x1f\x7f]/u.test(value)) return false;
  const bracketed = /^\[[^\]]+\]:(\d+)$/.exec(value);
  if (bracketed !== null) return validForwardPort(bracketed[1] ?? "");
  const separator = value.lastIndexOf(":");
  if (separator <= 0 || value.slice(0, separator).includes(":")) return false;
  return validForwardPort(value.slice(separator + 1));
}

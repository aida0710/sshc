import { directUserPattern, maxDirectDestinationLength, maxUserLength } from "../rules/generated";
import { isValidHostName } from "../rules/rules";

// DirectDestination is a place to connect to without saving it: what `ssh` takes on its command line. text is
// what the engine is asked to open (internal/validate/direct_destination.go reads the same form).
export type DirectDestination = {
  text: string;
  user: string;
  host: string;
  // port is left out when it is not written: the config files or the default 22 decide.
  port?: number;
};

// splitHostAndPort reads host[:port]. A bare IPv6 address (two or more colons) cannot carry a port, as for `ssh`.
function splitHostAndPort(text: string): { host: string; port?: string } | null {
  if (text.startsWith("[")) {
    const closing = text.indexOf("]");
    if (closing < 0) return null;
    const remainder = text.slice(closing + 1);
    if (remainder !== "" && !remainder.startsWith(":")) return null;
    return { host: text.slice(1, closing), ...(remainder === "" ? {} : { port: remainder.slice(1) }) };
  }
  const firstColon = text.indexOf(":");
  if (firstColon < 0 || text.indexOf(":", firstColon + 1) >= 0) return { host: text };
  return { host: text.slice(0, firstColon), port: text.slice(firstColon + 1) };
}

// parseDirectDestination reads user@host[:port], or answers null for anything else: most of what is typed in a
// search is a search.
export function parseDirectDestination(raw: string): DirectDestination | null {
  const text = raw.trim();
  const at = text.lastIndexOf("@");
  if (at < 0 || text.length > maxDirectDestinationLength) return null;
  const user = text.slice(0, at);
  if (user.length > maxUserLength || !directUserPattern.test(user)) return null;
  const split = splitHostAndPort(text.slice(at + 1));
  if (split === null || !isValidHostName(split.host)) return null;
  if (split.port === undefined) return { text, user, host: split.host };
  const port = /^\d{1,5}$/.test(split.port) ? Number(split.port) : 0;
  if (port < 1 || port > 65535) return null;
  return { text, user, host: split.host, port };
}

// sshSessionTarget says where an SSH session opens, in the terms the engine asks for it.
export function sshSessionTarget(name: string): { alias: string } | { destination: string } {
  return isDirectDestination(name) ? { destination: name } : { alias: name };
}

// isDirectDestination tells a session opened on a direct destination from one opened on a saved connection: a
// connection name cannot hold an @ (aliasPattern). Such a session has no connection name to read files or settings by.
export function isDirectDestination(alias: string): boolean {
  return alias.includes("@");
}

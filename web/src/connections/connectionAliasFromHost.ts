import { maxAliasLength } from "../rules/generated";
import { isValidAlias } from "../rules/rules";

const ipv4Address = /^\d{1,3}(\.\d{1,3}){3}$/;

// connectionAliasFromHostName is the connection name a host name suggests: 10.0.0.5 → 10-0-0-5 (an address reads the
// same whole, and dots in a Host name are easy to misread as a pattern), web.example.com → web. It is "" when the host
// name gives no valid name.
export function connectionAliasFromHostName(hostName: string): string {
  const isAddress = ipv4Address.test(hostName) || hostName.includes(":");
  const suggested = isAddress
    ? hostName.replace(/[.:]/g, "-").replace(/^-+|-+$/g, "")
    : hostName.split(".")[0] ?? "";
  const bounded = suggested.slice(0, maxAliasLength);
  return isValidAlias(bounded) ? bounded : "";
}

// uniqueConnectionAlias adds a number to base when a connection already has that name (web, web-2, web-3). Names are
// compared ignoring case, as ssh_config compares a Host pattern with the name it is given.
export function uniqueConnectionAlias(base: string, taken: readonly string[]): string {
  if (base === "") return "";
  const used = new Set(taken.map((alias) => alias.toLowerCase()));
  if (!used.has(base.toLowerCase())) return base;
  for (let number = 2; ; number += 1) {
    const suffix = `-${number}`;
    const candidate = base.slice(0, maxAliasLength - suffix.length) + suffix;
    if (!used.has(candidate.toLowerCase())) return candidate;
  }
}

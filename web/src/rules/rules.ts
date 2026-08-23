import {
  aliasPattern,
  groupSegmentPattern,
  hostnamePattern,
  maxAliasLength,
  maxGroupSegmentBytes,
  maxGroupSegments,
  maxHostnameLength,
  reservedNames,
} from "./generated";


function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function isValidGroupSegment(segment: string): boolean {
  if (byteLength(segment) > maxGroupSegmentBytes) return false;
  if (!groupSegmentPattern.test(segment)) return false;
  return !reservedNames.has(segment.toLowerCase());
}

export function isValidGroupName(name: string): boolean {
  if (name === "") return false;
  const segments = name.split("/");
  if (segments.length > maxGroupSegments) return false;
  return segments.every(isValidGroupSegment);
}

export function isValidAlias(alias: string): boolean {
  if (alias === "" || byteLength(alias) > maxAliasLength) return false;
  return aliasPattern.test(alias);
}

export function isValidHostName(value: string): boolean {
  if (value.length === 0 || byteLength(value) > maxHostnameLength) return false;
  return value.includes(":") ? isValidIPv6(value) : hostnamePattern.test(value);
}

function isValidIPv4(value: string): boolean {
  const parts = value.split(".");
  if (parts.length !== 4) return false;
  return parts.every((part) => {
    if (!/^\d{1,3}$/.test(part)) return false;
    if (part.length > 1 && part.startsWith("0")) return false;
    return Number(part) <= 255;
  });
}

function isValidIPv6(value: string): boolean {
  if (value.includes("%")) return false;
  let expanded = value;
  if (value.includes(".")) {
    const separator = value.lastIndexOf(":");
    if (separator < 0 || !isValidIPv4(value.slice(separator + 1))) return false;
    expanded = `${value.slice(0, separator)}:0:0`;
  }
  const compression = expanded.indexOf("::");
  if (compression !== expanded.lastIndexOf("::")) return false;
  const compressed = compression >= 0;
  const sides = compressed ? expanded.split("::") : [expanded];
  if (sides.some((side) => side !== "" && side.split(":").some((part) => !/^[0-9A-Fa-f]{1,4}$/.test(part)))) {
    return false;
  }
  const groups = sides.reduce((total, side) => total + (side === "" ? 0 : side.split(":").length), 0);
  return compressed ? groups < 8 : groups === 8;
}

export function formatValues(values: readonly string[]): string | null {
  if (values.some((value) => /[\n\r"\0]/.test(value))) return null;
  return values
    .map((value) => (value === "" || /[ \t]/.test(value) || value.startsWith("#") ? `"${value}"` : value))
    .join(" ");
}

export function parseValues(text: string): string[] {
  const values: string[] = [];
  let index = 0;
  while (index < text.length) {
    while (index < text.length && (text[index] === " " || text[index] === "\t")) index += 1;
    if (index >= text.length) break;

    if (text[index] === '"') {
      const closing = text.indexOf('"', index + 1);
      if (closing < 0) throw new Error("unbalanced_quote");
      values.push(text.slice(index + 1, closing));
      index = closing + 1;
      if (index < text.length && text[index] !== " " && text[index] !== "\t") {
        throw new Error("unbalanced_quote");
      }
      continue;
    }

    let end = index;
    while (end < text.length && text[end] !== " " && text[end] !== "\t") {
      if (text[end] === '"') throw new Error("unbalanced_quote");
      end += 1;
    }
    values.push(text.slice(index, end));
    index = end;
  }
  return values;
}

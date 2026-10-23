export type TerminalLinkMatch = {
  // A relative path is relative to the directory the program that printed it
  // works in; it is resolved when it is opened, not when it is found.
  kind: "url" | "path" | "relative-path";
  text: string;
  target: string;
  start: number;
  end: number;
};

export function isSafeHttpURL(value: string): boolean {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function modifierOpensLink(event: Pick<MouseEvent, "ctrlKey" | "metaKey">): boolean {
  return event.ctrlKey || event.metaKey;
}

// An OSC 8 link shows one text and opens another address. A modifier-click
// opens it at once only when the text names that same address; otherwise the
// address is shown first, so a program cannot dress one site up as another.
export function linkTextNamesTarget(link: Pick<TerminalLinkMatch, "text" | "target">): boolean {
  try {
    return new URL(link.text.trim()).href === new URL(link.target).href;
  } catch {
    return false;
  }
}

export function osc8Link(target: string, visibleText: string, start: number, end: number): TerminalLinkMatch | null {
  if (!isSafeHttpURL(target) || target.length > maxLinkLength) return null;
  return { kind: "url", text: visibleText || target, target, start, end };
}

const maxLinkLength = 4096;
const urlPattern = /https?:\/\/[^\s<>"'`]+/giu;
const pathPattern = /(?:^|[\s([{=])((?:\/[A-Za-z0-9._~@%+,:=-]+)+(?::\d+(?::\d+)?)?)/gu;
// Only images are taken from relative paths: an ordinary word such as
// "config.yaml" or "src/main.go" in program output is far more often prose than
// a file to open. Claude Code names an image it sent relative to its directory.
const relativeImagePattern = /(?:^|[\s([{=])((?:\.{1,2}\/)?(?:[A-Za-z0-9._~@%+,=-]+\/)*[A-Za-z0-9._~@%+,=-]+\.(?:png|jpe?g|gif|webp|bmp))(?!\.?[A-Za-z0-9_~@%+=/-])/giu;

function withoutTrailingPunctuation(value: string): string {
  let end = value.length;
  while (end > 0 && /[.,;!?)]/u.test(value[end - 1] ?? "")) end -= 1;
  return value.slice(0, end);
}

function pathTarget(value: string): string {
  return value.replace(/:\d+(?::\d+)?$/u, "");
}

function overlaps(matches: TerminalLinkMatch[], start: number, end: number): boolean {
  return matches.some((match) => start < match.end && end > match.start);
}

export function findTerminalLinks(line: string): TerminalLinkMatch[] {
  const matches: TerminalLinkMatch[] = [];
  for (const found of line.matchAll(urlPattern)) {
    const original = found[0] ?? "";
    const text = withoutTrailingPunctuation(original);
    const start = found.index ?? -1;
    if (start < 0 || text === "" || text.length > maxLinkLength) continue;
    matches.push({ kind: "url", text, target: text, start, end: start + text.length });
  }
  for (const found of line.matchAll(pathPattern)) {
    const original = found[1] ?? "";
    const text = withoutTrailingPunctuation(original);
    const whole = found[0] ?? "";
    const prefix = Math.max(0, whole.length - original.length);
    const start = (found.index ?? -1) + prefix;
    const target = pathTarget(text);
    const end = start + text.length;
    if (start < 0 || target === "" || target.length > maxLinkLength || overlaps(matches, start, end)) continue;
    matches.push({ kind: "path", text, target, start, end });
  }
  for (const found of line.matchAll(relativeImagePattern)) {
    const text = found[1] ?? "";
    const whole = found[0] ?? "";
    const start = (found.index ?? -1) + whole.length - text.length;
    const end = start + text.length;
    if (start < 0 || text.length > maxLinkLength || overlaps(matches, start, end)) continue;
    // Named users require a different home lookup; do not offer a wrong path.
    if (text.startsWith("~") && !text.startsWith("~/")) continue;
    matches.push({ kind: "relative-path", text, target: text, start, end });
  }
  return matches.sort((left, right) => left.start - right.start || left.end - right.end);
}

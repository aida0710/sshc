export type TerminalLinkMatch = {
  kind: "url" | "remote-path";
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

export function osc8Link(target: string, visibleText: string, start: number, end: number): TerminalLinkMatch | null {
  if (!isSafeHttpURL(target) || target.length > maxLinkLength) return null;
  return { kind: "url", text: visibleText || target, target, start, end };
}

const maxLinkLength = 4096;
const urlPattern = /https?:\/\/[^\s<>"'`]+/giu;
const pathPattern = /(?:^|[\s([{=])((?:\/[A-Za-z0-9._~@%+,:=-]+)+(?::\d+(?::\d+)?)?)/gu;

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

export function findTerminalLinks(line: string, remote: boolean): TerminalLinkMatch[] {
  const matches: TerminalLinkMatch[] = [];
  for (const found of line.matchAll(urlPattern)) {
    const original = found[0] ?? "";
    const text = withoutTrailingPunctuation(original);
    const start = found.index ?? -1;
    if (start < 0 || text === "" || text.length > maxLinkLength) continue;
    matches.push({ kind: "url", text, target: text, start, end: start + text.length });
  }
  if (remote) {
    for (const found of line.matchAll(pathPattern)) {
      const original = found[1] ?? "";
      const text = withoutTrailingPunctuation(original);
      const whole = found[0] ?? "";
      const prefix = Math.max(0, whole.length - original.length);
      const start = (found.index ?? -1) + prefix;
      const target = pathTarget(text);
      const end = start + text.length;
      if (start < 0 || target === "" || target.length > maxLinkLength || overlaps(matches, start, end)) continue;
      matches.push({ kind: "remote-path", text, target, start, end });
    }
  }
  return matches.sort((left, right) => left.start - right.start || left.end - right.end);
}

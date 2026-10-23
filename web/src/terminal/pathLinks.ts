import type { TerminalLinkMatch } from "./links";

// Relative names are offered only for images, so ordinary source-file names
// in prose do not become links. The same targets feed preview navigation.
export const imageNamePattern = /\.(?:png|jpe?g|gif|webp|bmp)$/iu;
const windowsPathPattern = /^[A-Za-z]:[/\\]/u;
const sourceLocationPattern = /:\d+(?::\d+)?$/u;
const homePrefixPattern = /^\$(?:HOME|\{HOME\})\//u;
const trailingPunctuationPattern = /[.,;!?)\]}。、）]+$/u;

export function withoutTrailingPunctuation(value: string, kind: "url" | "path"): string {
  // A closing bracket may wrap a printed path, but also belongs to IPv6 URLs.
  const pattern = kind === "path" ? trailingPunctuationPattern : /[.,;!?)。、）]+$/u;
  return value.replace(pattern, "");
}

type PrintedWord = { text: string; start: number; protected: boolean; complete: boolean };

// Consume the whole shell word, including unsupported syntax, before deciding
// whether it is a path. Retrying inside a rejected word would create fragments.
function printedWords(line: string): PrintedWord[] {
  const words: PrintedWord[] = [];
  let index = 0;
  while (index < line.length) {
    if (/\s/u.test(line[index]!)) { index += 1; continue; }
    const start = index;
    let quote = "";
    let protectedWord = false;
    while (index < line.length) {
      const character = line[index]!;
      if (quote === "" && /\s/u.test(character)) break;
      if (character === "\\" && quote !== "'") {
        protectedWord = true;
        index += Math.min(2, line.length - index);
        continue;
      }
      if (character === quote) quote = "";
      else if (quote === "" && /["'`]/u.test(character)) {
        quote = character;
        protectedWord = true;
      }
      index += 1;
    }
    words.push({ text: line.slice(start, index), start, protected: protectedWord, complete: quote === "" });
  }
  return words;
}

function decodedPath(text: string): string {
  const quote = /^["'`]/u.exec(text)?.[0];
  const contents = quote !== undefined && text.endsWith(quote) ? text.slice(1, -1) : text;
  if (windowsPathPattern.test(contents)) return contents.replaceAll("\\", "/");
  if (quote === "'") return contents;
  // Double quotes preserve backslashes except before shell-special characters.
  return quote === '"'
    ? contents.replace(/\\([\\"$`])/gu, "$1")
    : contents.replace(/\\([^\r\n])/gu, "$1");
}

function fileURIPath(value: string, allowWindowsPaths: boolean): string | null {
  // A host component would name another machine. Reading that path from this
  // session would silently open a different file; only local file URIs qualify.
  if (!value.startsWith("file:///")) return null;
  try {
    const location = new URL(value);
    if (location.host !== "" || location.search !== "" || location.hash !== "") return null;
    const path = decodeURIComponent(location.pathname);
    if (/^\/[A-Za-z]:\//u.test(path)) return allowWindowsPaths ? path.slice(1) : null;
    return path;
  } catch { return null; }
}

function wordPath(word: PrintedWord, allowWindowsPaths: boolean): TerminalLinkMatch | null {
  if (!word.complete) return null;
  let text = withoutTrailingPunctuation(word.text, "path");
  let start = word.start;
  // Labels may touch an explicit path, but foo:bar.png and URI path segments
  // are whole words, not relative files. Drive letters must be checked first.
  if (!windowsPathPattern.test(text) && !/^["'`]/u.test(text) && !/^[a-z][a-z\d+.-]*:\/\//iu.test(text)) {
    const prefix = /(?:^|[:：=→←↑↓⇒➜▶([{<])(?=[A-Za-z]:[/\\]|\/|~\/|\$(?:HOME|\{HOME\})\/)/u.exec(text);
    const firstSlash = text.search(/[/\\]/u);
    const hasPrefix = prefix !== null && (firstSlash < 0 || prefix.index < firstSlash);
    const offset = hasPrefix ? prefix.index + prefix[0].length : (/^[([{<→←]+/u.exec(text)?.[0].length ?? 0);
    text = text.slice(offset);
    start += offset;
  }
  let target = decodedPath(text);
  // Keep the underline on the printed path, including escapes, not its quotes.
  if (/^["'`]/u.test(text) && text.endsWith(text[0]!)) {
    text = text.slice(1, -1);
    start += 1;
  }
  if (text === "" || /[\p{C}]/u.test(target)) return null;
  if (target.startsWith("file:")) {
    const path = fileURIPath(target.replace(sourceLocationPattern, ""), allowWindowsPaths);
    if (path === null) return null;
    target = path;
  } else {
    target = target.replace(sourceLocationPattern, "").replace(homePrefixPattern, "~/");
    if (windowsPathPattern.test(target)) {
      if (!allowWindowsPaths) return null;
    } else if ((target.includes(":") && !target.startsWith("/")) || target.includes("\\") || /["'`$]/u.test(target)) return null;
  }
  if (target.startsWith("//") || /[\p{C}]/u.test(target)) return null;
  if (target.startsWith("/") || windowsPathPattern.test(target)) {
    return { kind: "path", text, target, start, end: start + text.length };
  }
  if ((target.startsWith("~") && !target.startsWith("~/")) || !imageNamePattern.test(target)) return null;
  return { kind: "relative-path", text, target, start, end: start + text.length };
}

// Without quotes or escapes, a path followed by filename-like words could be
// one path containing spaces. A complete image name already ends a candidate;
// otherwise prefer no link to opening only its head or tail.
function ambiguousPathEnd(words: PrintedWord[], index: number): number {
  const first = words[index]!;
  if (imageNamePattern.test(first.text.replace(sourceLocationPattern, ""))) return index;
  const windows = windowsPathPattern.test(first.text);
  if (/^["'`]/u.test(first.text) || (!windows && !first.text.includes("/")) || trailingPunctuationPattern.test(first.text)) return index;
  for (let next = index + 1; next < words.length; next += 1) {
    const word = words[next]!;
    const text = withoutTrailingPunctuation(word.text, "path").replace(sourceLocationPattern, "");
    if (word.protected || /^(?:\/|~|\$|\.{1,2}\/)/u.test(text) || /[:：=()[\]{}<>→←,;!?。、）]/u.test(text)) break;
    if (/\.[\p{L}\p{N}]+$/u.test(text)) return next;
    if (text.includes("/") || text !== word.text) break;
  }
  return index;
}

export function findPathLinks(line: string, allowWindowsPaths: boolean): TerminalLinkMatch[] {
  const words = printedWords(line);
  const matches: TerminalLinkMatch[] = [];
  for (let index = 0; index < words.length; index += 1) {
    const word = words[index]!;
    const ambiguousEnd = ambiguousPathEnd(words, index);
    if (ambiguousEnd > index) { index = ambiguousEnd; continue; }
    const match = wordPath(word, allowWindowsPaths);
    if (match !== null) matches.push(match);
  }
  return matches;
}

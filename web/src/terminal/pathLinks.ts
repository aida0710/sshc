import type { TerminalLinkMatch } from "./links";

// Relative names are offered only when they end in a known file extension,
// so words that merely contain a slash ("and/or") do not become links. The
// list covers what programs such as Claude Code hand back: images, text and
// source, documents, audio, video and archives. The same targets feed
// preview navigation.
const previewableExtensions = [
  "png", "jpg", "jpeg", "gif", "webp", "bmp", "svg",
  "md", "markdown", "txt", "log", "csv", "tsv", "json", "jsonl", "yaml", "yml", "toml", "ini", "conf", "xml",
  "html", "htm", "css", "js", "mjs", "cjs", "ts", "tsx", "jsx", "py", "rb", "go", "rs", "java", "kt", "swift",
  "c", "h", "cc", "cpp", "hpp", "cs", "php", "sh", "bash", "zsh", "fish", "ps1", "sql", "lua", "r", "dart",
  "scala", "vue", "svelte", "env", "diff", "patch",
  "pdf",
  "mp3", "wav", "m4a", "aac", "ogg", "oga", "opus", "flac",
  "mp4", "m4v", "webm", "mov", "mkv", "ogv",
  "zip", "tar", "gz", "tgz", "xz", "7z",
];
export const previewableNamePattern = new RegExp(`\\.(?:${previewableExtensions.join("|")})$`, "iu");
// Japanese prose puts no space around a path, so a name with a known extension may run straight into the
// words after it ("shot.pngに保存しました", "a.png（PC）"). Such a path ends at its extension, with a
// :line:column and a closing quote kept, where a non-ASCII character follows. ASCII may go on a path, a query
// or a longer name ("x.png.bak"), so it never ends one.
const previewableNameBeforeProse = new RegExp(
  `\\.(?:${previewableExtensions.join("|")})(?::\\d+(?::\\d+)?)?["'\`]?(?=\\P{ASCII})`,
  "iu",
);
// The prose before a path can touch it too ("画像は~/shots/a.png"). A home path cannot start in the middle of a
// name, so it starts after the prose. A path from the root is told from a relative folder with a Unicode name
// ("画像/a.png") only by starting at one of the folders an absolute path usually starts with.
const pathAfterProse = /\P{ASCII}(?=~\/|\$(?:HOME|\{HOME\})\/|\/(?:home|Users|tmp|var|etc|opt|usr|srv|mnt|root|private|Volumes)\/)/u;
// A tool call such as Claude Code's Read(src/app.ts), or a Japanese bracket, opens right before a relative
// path. The bracket must stay open up to the first slash, so a folder named "dir(1)" is kept whole.
const openingBeforeRelativePath = /^[^/\\]*[([{<（「『](?=[^/\\)\]}>）」』]*[/\\])/u;
// The prose before a relative name touches it too ("確かめました。xterm.js", "修正済みのREADME.mdを読んだ"). A name
// starts after the last sentence or clause mark, which no file or folder name holds. A name without a folder also
// starts after a bracket ("「xterm.js」"; a folder may be named "写真（2026）"), and where ASCII that begins with a
// letter takes over from Japanese words up to its end. A name that begins with a digit or a dot keeps the words
// before it ("議事録2026-11-01.md", "運用メモ.md").
const sentenceMarkBeforePath = /^.*[。、，．！？：；]/u;
const bracketBeforeName = /^.*[（）「」『』【】〔〕]/u;
const asciiNameAfterProse = /^.*\P{ASCII}(?=[A-Za-z][\x21-\x7e]*$)/u;
const windowsPathPattern = /^[A-Za-z]:[/\\]/u;
const sourceLocationPattern = /:\d+(?::\d+)?$/u;
const homePrefixPattern = /^\$(?:HOME|\{HOME\})\//u;
const trailingPunctuationPattern = /[.,;!?)\]}。、）]+$/u;

// proseBeforePath is how many characters at the start of text are the prose before the path it ends with (see
// sentenceMarkBeforePath).
function proseBeforePath(text: string): number {
  const marked = sentenceMarkBeforePath.exec(text)?.[0].length ?? 0;
  const rest = text.slice(marked);
  if (/[/\\]/u.test(rest)) return marked;
  const bracketed = bracketBeforeName.exec(rest)?.[0].length ?? 0;
  return marked + bracketed + (asciiNameAfterProse.exec(rest.slice(bracketed))?.[0].length ?? 0);
}

// endBeforeProse cuts the words that follow a previewable name without a space (see previewableNameBeforeProse).
function endBeforeProse(text: string): string {
  if (previewableNamePattern.test(text.replace(sourceLocationPattern, ""))) return text;
  const found = previewableNameBeforeProse.exec(text);
  return found === null ? text : text.slice(0, found.index + found[0].length);
}

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
  let text = endBeforeProse(withoutTrailingPunctuation(word.text, "path"));
  let start = word.start;
  // Labels may touch an explicit path, but foo:bar.png and URI path segments
  // are whole words, not relative files. Drive letters must be checked first.
  if (!windowsPathPattern.test(text) && !/^["'`]/u.test(text) && !/^[a-z][a-z\d+.-]*:\/\//iu.test(text)) {
    const prefix = /(?:^|[:：=→←↑↓⇒➜▶([{<（「『])(?=[A-Za-z]:[/\\]|\/|~\/|\$(?:HOME|\{HOME\})\/)/u.exec(text) ?? pathAfterProse.exec(text);
    const firstSlash = text.search(/[/\\]/u);
    const hasPrefix = prefix !== null && (firstSlash < 0 || prefix.index < firstSlash);
    const offset = hasPrefix
      ? prefix.index + prefix[0].length
      : (openingBeforeRelativePath.exec(text) ?? /^[([{<→←]+/u.exec(text))?.[0].length ?? 0;
    text = text.slice(offset);
    start += offset;
    if (!hasPrefix) {
      const prose = proseBeforePath(text);
      text = text.slice(prose);
      start += prose;
    }
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
  if ((target.startsWith("~") && !target.startsWith("~/")) || !previewableNamePattern.test(target)) return null;
  return { kind: "relative-path", text, target, start, end: start + text.length };
}

// Without quotes or escapes, a path followed by filename-like words could be
// one path containing spaces. A complete previewable name already ends a candidate;
// otherwise prefer no link to opening only its head or tail.
function ambiguousPathEnd(words: PrintedWord[], index: number): number {
  const first = words[index]!;
  if (endBeforeProse(first.text) !== first.text || previewableNamePattern.test(first.text.replace(sourceLocationPattern, ""))) return index;
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

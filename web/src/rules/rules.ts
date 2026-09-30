import {
  aliasPattern,
  groupSegmentPattern,
  hostnamePattern,
  maxAliasLength,
  maxGroupSegmentBytes,
  maxGroupSegments,
  maxHostnameLength,
  reservedNames,
  restOfLineKeywords,
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

// formatValues は、Go の config.RenderArgument と同じ表記で値を並べる。
// OpenSSH の argv_split が同じ値に読み戻す書き方で、書けない値（改行と NUL）が
// あれば null を返す。
export function formatValues(values: readonly string[]): string | null {
  if (values.some((value) => /[\n\r\0]/.test(value))) return null;
  return values.map((value) => (needsQuoting(value) ? quoteValue(value) : value)).join(" ");
}

function needsQuoting(value: string): boolean {
  return (
    value === "" ||
    /[ \t"']/.test(value) ||
    value.startsWith("#") ||
    value.startsWith("=") ||
    value.endsWith("\\") ||
    value.includes("\\\\")
  );
}

// quoteValue は二重引用符で囲む。引用の中で argv_split がエスケープと読む形になる
// バックスラッシュと、末尾のバックスラッシュだけを二重にする。
function quoteValue(value: string): string {
  let quoted = '"';
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index]!;
    const next = value[index + 1];
    if (character === '"') {
      quoted += '\\"';
    } else if (character === "\\" && (next === undefined || next === "\\" || next === '"' || next === "'")) {
      quoted += "\\\\";
    } else {
      quoted += character;
    }
  }
  return `${quoted}"`;
}

// formatDirectiveValues は、keyword の行の値を画面で編集する文字列にする。
//
// ProxyCommand などの行の残りを値にするキーワードは、サーバーが書かれたとおりの
// 行の残りをひとつの値で渡す。値を読むのはシェルなので、引用し直さずにそのまま見せる。
export function formatDirectiveValues(keyword: string, values: readonly string[]): string | null {
  if (restOfLineKeywords.has(keyword.toLowerCase())) return values.join(" ");
  return formatValues(values);
}

// parseDirectiveValues は、画面で編集した文字列を keyword の行の値に戻す。
//
// 行の残りを値にするキーワードは、引数に分けずにひとつの値で送る。サーバーはそれを
// 引用せずに書く。閉じない引用は、OpenSSH が「invalid quotes」として設定ごと断るので
// ここで断る（unbalanced_quote）。サーバーと同じく、行末のコメントの中の引用符は数えない。
export function parseDirectiveValues(keyword: string, text: string): string[] {
  if (!restOfLineKeywords.has(keyword.toLowerCase())) return parseValues(text);
  const restOfLine = text.replace(/^[ \t]+|[ \t]+$/g, "");
  if (restOfLine === "") return [];
  splitWords(restOfLine, { commentEndsTheLine: true });
  return [restOfLine];
}

// parseValues は、Go の config パッケージと同じく OpenSSH の argv_split の規則で
// 値を分ける。引用は一重・二重とも語のどこでも開閉し、バックスラッシュは \" \' \\ と
// 引用の外の "\ " だけをエスケープとして読む。閉じない引用は unbalanced_quote。
export function parseValues(text: string): string[] {
  return splitWords(text, { commentEndsTheLine: false });
}

// splitWords は、argv_split の規則で text を語に分ける。commentEndsTheLine なら、
// Go の config.splitArguments と同じく、語の先頭の '#' から後ろをコメントとして読まない。
function splitWords(text: string, { commentEndsTheLine }: { commentEndsTheLine: boolean }): string[] {
  const values: string[] = [];
  let index = 0;
  while (index < text.length) {
    while (index < text.length && isSpace(text[index]!)) index += 1;
    if (index >= text.length) break;
    if (commentEndsTheLine && text[index] === "#") break;

    let value = "";
    let quote = "";
    while (index < text.length) {
      const character = text[index]!;
      const next = text[index + 1];
      if (character === "\\" && next !== undefined && isEscaped(next, quote)) {
        value += next;
        index += 2;
        continue;
      }
      if (quote === "" && isSpace(character)) break;
      if (quote === "" && (character === '"' || character === "'")) {
        quote = character;
      } else if (quote !== "" && character === quote) {
        quote = "";
      } else {
        value += character;
      }
      index += 1;
    }
    if (quote !== "") throw new Error("unbalanced_quote");
    values.push(value);
  }
  return values;
}

function isSpace(character: string): boolean {
  return character === " " || character === "\t";
}

function isEscaped(next: string, quote: string): boolean {
  if (next === "'" || next === '"' || next === "\\") return true;
  return next === " " && quote === "";
}

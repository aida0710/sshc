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

// ここにあるのは、サーバーと同じ答えを出したい判断である。
//
// **守る契約は「サーバーより厳しくしない」。** 緩い側にずれても、サーバーが正しく
// 断り、利用者は理由を受け取る。厳しい側にずれると**正しい入力が画面で止められる**
// ——そちらは利用者に直しようがない。rules.test.ts が、生成されたコーパスで
// その向きだけを赤にする。
//
// 表（予約語・パターン・上限）は generated.ts から来る。**手で書き写さない** ——
// 予約語が Go に 10、画面に 6 あった期間、`rc` や `environment` は画面が緑を出して
// サーバーが断っていた。

// byteLength は、Go が数えるのと同じ単位で長さを数える。
//
// Go の len() はバイトを数え、JavaScript の .length は UTF-16 の単位を数える。
// 受理される綴りは ASCII に限られるので実際には一致するが、**上限の判断で
// 食い違わないよう**、こちらもバイトで数える。
function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function isValidGroupSegment(segment: string): boolean {
  if (byteLength(segment) > maxGroupSegmentBytes) return false;
  if (!groupSegmentPattern.test(segment)) return false;
  // **鍵のファイル名にも同じ一覧が効く。** どちらも ~/.ssh の直下にその綴りを
  // 作る操作である。
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

// isValidHostName は、DNS 名・IPv4・角括弧なしの IPv6 を受け付ける。
//
// **ここだけは表から作れない。** Go は net.ParseIP に渡しており、あれと同じ答えを
// 出す正規表現は無い。だから手で書くしかなく、**だからこそコーパスで縛る** ——
// zone 識別子や IPv4 射影の先頭ゼロのように Go が断るところで画面が緑を出しても
// 契約は破れないが、Go が通すところで画面が断てば赤くなる。
export function isValidHostName(value: string): boolean {
  if (value.length === 0 || byteLength(value) > maxHostnameLength) return false;
  return value.includes(":") ? isValidIPv6(value) : hostnamePattern.test(value);
}

function isValidIPv4(value: string): boolean {
  const parts = value.split(".");
  if (parts.length !== 4) return false;
  return parts.every((part) => {
    // **先頭ゼロを断る。** net.ParseIP はそれを 8 進と読まず、単に拒む。
    if (!/^\d{1,3}$/.test(part)) return false;
    if (part.length > 1 && part.startsWith("0")) return false;
    return Number(part) <= 255;
  });
}

function isValidIPv6(value: string): boolean {
  // zone 識別子は綴りとして受け付けない。net.ParseIP も受け付けない。
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

// formatValues は、値の並びを ssh_config の 1 行として書き出す。綴りが無ければ
// null を返す。
//
// **OpenSSH は引用された引数の中にバックスラッシュエスケープを持たない。** だから
// 二重引用符・改行・NUL を含む値には書き方が無く、Go の RenderArgument は壊すのでは
// なく ErrUnquotableValue で断る。ここも同じものを断る——**返す型が null を含むのは、
// その規則があることを読む側に隠さないためである。** 以前ここは黙って壊れた引用を
// 生成しており、画面は保存できたように見せて、サーバーが invalid_request を返した。
//
// 空白は綴りがある（引用で囲める）ので、断る対象ではない。
export function formatValues(values: readonly string[]): string | null {
  if (values.some((value) => /[\n\r"\0]/.test(value))) return null;
  return values
    .map((value) => (value === "" || /[ \t]/.test(value) || value.startsWith("#") ? `"${value}"` : value))
    .join(" ");
}

// parseValues は、書かれた 1 行を値の並びとして読む。
//
// OpenSSH の argv_split は、トークンの先頭にある二重引用符を次の二重引用符までの
// 引用文字列の始まりとして扱い、バックスラッシュエスケープには対応しない。**その
// 規則で読めない行は断る** ——Go はそれを非構造化として逐語で保つので、画面が
// 勝手に意味を推測してはならない。
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

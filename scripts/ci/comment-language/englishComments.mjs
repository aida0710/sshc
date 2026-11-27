// 変更で足したコメントの行のうち、日本語で書いていない行を探す。sshc のコードのコメントは日本語で
// 書く決まりで、見るのは言語だけである。日本語の良し悪しは見ない。
//
// 英語の行とみなすのは、かなも漢字も含まず、英単語が englishRunLength 個以上並ぶ行である。英単語は
// 空白で区切った語のうち英字だけのもので、識別子（foo.bar()、snake_case）や URL は英単語に数えない。
// 次の行は英語でも通す。
// - 同じ変更で消したコメントの行と同じ文の行。ほかの所から移しただけのコメントである。
// - 指示の行（//go:build、// oxlint-disable-next-line など）。書き方がツールで決まっている。
// - コメントの印の直後がタブの行。Go の文書コメントのコードの例の書き方である。
// - exemptionMarker と日本語の理由を書いた行に続く、コメントだけの行の並び。英語の仕様の引用など。

import { commentLinesOf } from "./commentLines.mjs";

// 英単語がこの数まで並んだら、英語の文とみなす。2つまでの並びは、「ssh -J」の ssh や
// 「known_hosts の host key」の host key のように、日本語の文の中に置く語の断片に多い。
const englishRunLength = 3;

const japaneseCharacter = /[\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Han}]/u;

// 語の前後に付く括弧・引用符・句読点。これらを除いてから、英字だけの語かを見る。
const surroundingPunctuation = /^[("'`「『[{<]+|[)"'`」』\]}>.,;:!?]+$/gu;
const englishWord = /^[A-Za-z]+(?:['’-][A-Za-z]+)*$/;

// 書き方がツールで決まっている指示の頭。Go のコンパイラ（go:）、golangci-lint（nolint）、
// staticcheck（lint:ignore）、ESLint・Oxlint、TypeScript（@ts-expect-error など）が読む。
const directivePrefixes = ["go:", "nolint", "lint:", "eslint-", "oxlint-", "@ts-"];

// この印と日本語の理由を書いた行に続く、コメントだけの行の並びは、英語でも通す。
export const exemptionMarker = "comment-language-exempt";
const exemption = new RegExp(`${exemptionMarker}:(.*)$`);

// textOf は、コメントの断片から、// や /* */、行の頭の * などの印を除いた文を返す。
function textOf(piece) {
  return piece
    .trim()
    .replace(/^\/\/+/, "")
    .replace(/^\/\*+/, "")
    .replace(/\*+\/$/, "")
    .replace(/^\*+/, "")
    .trim();
}

function lineTextOf({ pieces }) {
  return pieces.map(textOf).join(" ").replace(/\s+/g, " ").trim();
}

function longestEnglishRun(text) {
  let longest = 0;
  let run = 0;
  for (const word of text.split(/\s+/)) {
    run = englishWord.test(word.replace(surroundingPunctuation, "")) ? run + 1 : 0;
    longest = Math.max(longest, run);
  }
  return longest;
}

// isEnglish は、コメントの文が、日本語を含まない英語の文かを返す。
export function isEnglish(text) {
  return !japaneseCharacter.test(text) && longestEnglishRun(text) >= englishRunLength;
}

function isDirective(text) {
  return directivePrefixes.some((prefix) => text.startsWith(prefix));
}

function isCodeExample({ pieces }) {
  return pieces.every((piece) => /^\s*(?:\/\/|\/\*+|\*)\t/.test(piece));
}

// opensExemption は、その行に、日本語の理由を付けた exemptionMarker があるかを返す。
function opensExemption(text) {
  const reason = text.match(exemption)?.[1];
  return reason !== undefined && japaneseCharacter.test(reason);
}

// exemptLinesOf は、exemptionMarker の行に続く、コメントだけの行の番号を返す。
function exemptLinesOf(commentLines) {
  const exempt = new Set();
  const numbers = [...commentLines.keys()].sort((left, right) => left - right);
  let exemptingFrom;
  for (const number of numbers) {
    const line = commentLines.get(number);
    if (exemptingFrom === number && line.commentOnly) {
      exempt.add(number);
      exemptingFrom = number + 1;
    }
    if (opensExemption(lineTextOf(line))) {
      exemptingFrom = number + 1;
    }
  }
  return exempt;
}

// removedCommentTextsOf は、変更で消したコメントの行の文を、空白をそろえて返す。
function removedCommentTextsOf(changedFiles) {
  const texts = new Set();
  for (const { language, oldSource, removedLines } of changedFiles) {
    if (oldSource === undefined) {
      continue;
    }
    const commentLines = commentLinesOf(oldSource, language);
    for (const number of removedLines) {
      const line = commentLines.get(number);
      if (line !== undefined) {
        texts.add(lineTextOf(line));
      }
    }
  }
  return texts;
}

// findEnglishComments は、変更したファイルの、足したコメントの行のうち英語の行を返す。
// changedFiles の各要素は、path、language（commentLines.mjs の言語）、変更の前と後の中身
// （oldSource・newSource。無いファイルは undefined）、消した行と足した行の番号（removedLines・
// addedLines。1 から数える）を持つ。
export function findEnglishComments(changedFiles) {
  const removedTexts = removedCommentTextsOf(changedFiles);
  const findings = [];
  for (const { path, language, newSource, addedLines } of changedFiles) {
    if (newSource === undefined) {
      continue;
    }
    const commentLines = commentLinesOf(newSource, language);
    const exempt = exemptLinesOf(commentLines);
    for (const number of [...addedLines].sort((left, right) => left - right)) {
      const line = commentLines.get(number);
      if (line === undefined || exempt.has(number) || isCodeExample(line)) {
        continue;
      }
      const text = lineTextOf(line);
      if (isDirective(text) || removedTexts.has(text) || !isEnglish(text)) {
        continue;
      }
      findings.push({ path, line: number, text });
    }
  }
  return findings;
}

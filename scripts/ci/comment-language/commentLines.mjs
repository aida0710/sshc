// ソースのコメントを、言語ごとの字句の決まりで探し、行ごとに取り出す。文字列・テンプレート・
// 正規表現の中の // や /* はコメントにしない。
//
// 構文を解析せずに字句だけを読むので、迷う所はコメントにしない側に倒す。コメントを見落としても
// 英語のコメントが1行通るだけだが、コメントでない所をコメントとみなすと、日本語で書いた人の変更が
// 落ちる。そのため、' と " の文字列は行の終わりで打ち切り、閉じ忘れの影響を次の行に広げない。

// literal は、文字列のような、中の // をコメントにしない字句の決まりである。
// escapes はバックスラッシュで次の字を飛ばすか、multiline は行をまたげるか、templates は ${ … } の
// 中にコードを書けるかを表す。
function literal(open, { close = open, escapes = true, multiline = false, templates = false } = {}) {
  return { open, close, escapes, multiline, templates };
}

// 言語ごとの字句の決まり。literals は開く印の長いものを先に並べる（""" を " より先に見るため）。
const languages = {
  go: {
    literals: [literal('"'), literal("'"), literal("`", { escapes: false, multiline: true })],
  },
  java: {
    literals: [literal('"""', { multiline: true }), literal('"'), literal("'")],
  },
  kotlin: {
    // Kotlin のブロックコメントは入れ子にできる。` で囲む名前（テストの名前など）には ' が入りうる。
    nestedBlockComments: true,
    literals: [
      literal('"""', { escapes: false, multiline: true, templates: true }),
      literal('"', { templates: true }),
      literal("'"),
      literal("`", { escapes: false }),
    ],
  },
  javascript: {
    regexLiterals: true,
    // .tsx の JSX の地の文は字句として読めないので、地の文の URL（https://…）をコメントと読まない。
    jsxText: true,
    literals: [literal('"'), literal("'"), literal("`", { multiline: true, templates: true })],
  },
};

const languageOfExtension = {
  ".go": "go",
  ".java": "java",
  ".kt": "kotlin",
  ".kts": "kotlin",
  ".js": "javascript",
  ".mjs": "javascript",
  ".ts": "javascript",
  ".tsx": "javascript",
};

// languageOfPath は、パスの拡張子から言語を返す。見ない拡張子なら undefined を返す。
export function languageOfPath(path) {
  const extension = path.match(/\.[^./]+$/)?.[0];
  return extension === undefined ? undefined : languageOfExtension[extension];
}

// この後に / が来たら正規表現が始まる、と読むキーワード。ほかの名前の後の / は割り算である。
const keywordsBeforeExpression = new Set([
  "await",
  "case",
  "delete",
  "do",
  "else",
  "in",
  "instanceof",
  "new",
  "of",
  "return",
  "throw",
  "typeof",
  "void",
  "yield",
]);

// この後の / を割り算と読む記号。< を入れるのは、JSX の閉じるタグ（</div>）を正規表現と読まないため。
const punctuationBeforeDivision = new Set([")", "]", "}", "<"]);

const wordCharacter = /[\p{L}\p{N}_$]/u;

function lineEndOf(source, position) {
  const newline = source.indexOf("\n", position);
  return newline < 0 ? source.length : newline;
}

// scanLiteral は、開く印の後の position から字句の終わりを探す。閉じる印の後か、テンプレートの
// ${ の後（opensExpression が true）か、行をまたげない字句なら行の終わりで止まる。
function scanLiteral(source, position, { close, escapes, multiline, templates }) {
  let index = position;
  while (index < source.length) {
    if (escapes && source[index] === "\\") {
      index += 2;
      continue;
    }
    if (source.startsWith(close, index)) {
      return { end: index + close.length, opensExpression: false };
    }
    if (templates && source.startsWith("${", index)) {
      return { end: index + 2, opensExpression: true };
    }
    if (!multiline && source[index] === "\n") {
      return { end: index, opensExpression: false };
    }
    index += 1;
  }
  return { end: source.length, opensExpression: false };
}

// blockCommentEnd は、/* の位置から、閉じる */ の後の位置を返す。閉じていなければ末尾を返す。
function blockCommentEnd(source, start, nested) {
  let depth = 0;
  let index = start;
  while (index < source.length) {
    if (source.startsWith("/*", index)) {
      depth = nested ? depth + 1 : 1;
      index += 2;
    } else if (source.startsWith("*/", index)) {
      depth -= 1;
      index += 2;
      if (depth === 0) {
        return index;
      }
    } else {
      index += 1;
    }
  }
  return source.length;
}

// regexEnd は、/ の位置から正規表現の終わり（フラグの後）を返す。行の中で閉じなければ、正規表現
// ではないとみなして undefined を返す。
function regexEnd(source, start) {
  let inClass = false;
  for (let index = start + 1; index < source.length; index += 1) {
    const character = source[index];
    if (character === "\n") {
      return undefined;
    }
    if (character === "\\") {
      index += 1;
    } else if (character === "[") {
      inClass = true;
    } else if (character === "]") {
      inClass = false;
    } else if (character === "/" && !inClass) {
      let end = index + 1;
      while (end < source.length && /[a-z]/i.test(source[end])) {
        end += 1;
      }
      return end;
    }
  }
  return undefined;
}

// findCommentRanges は、source のコメントの範囲を [start, end) の文字の位置で返す。
function findCommentRanges(source, language) {
  const { literals, nestedBlockComments = false, regexLiterals = false, jsxText = false } = languages[language];
  const ranges = [];
  // 開いているテンプレートの式（${ … }）ごとに、その字句と、式の中で開いている { の数を積む。
  const openExpressions = [];
  let regexMayStart = true;
  let position = source.startsWith("#!") ? lineEndOf(source, 0) : 0;

  const continueLiteral = (from, opened) => {
    const { end, opensExpression } = scanLiteral(source, from, opened);
    if (opensExpression) {
      openExpressions.push({ literal: opened, braces: 0 });
    }
    regexMayStart = opensExpression;
    return end;
  };

  while (position < source.length) {
    const character = source[position];
    if (source.startsWith("//", position)) {
      if (jsxText && source[position - 1] === ":") {
        position = lineEndOf(source, position);
        continue;
      }
      const end = lineEndOf(source, position);
      ranges.push({ start: position, end });
      position = end;
      continue;
    }
    if (source.startsWith("/*", position)) {
      const end = blockCommentEnd(source, position, nestedBlockComments);
      ranges.push({ start: position, end });
      position = end;
      continue;
    }
    const opened = literals.find((candidate) => source.startsWith(candidate.open, position));
    if (opened !== undefined) {
      position = continueLiteral(position + opened.open.length, opened);
      continue;
    }
    if (regexLiterals && character === "/" && regexMayStart) {
      const end = regexEnd(source, position);
      if (end !== undefined) {
        position = end;
        regexMayStart = false;
        continue;
      }
    }
    if (wordCharacter.test(character)) {
      let end = position + 1;
      while (end < source.length && wordCharacter.test(source[end])) {
        end += 1;
      }
      regexMayStart = keywordsBeforeExpression.has(source.slice(position, end));
      position = end;
      continue;
    }
    const expression = openExpressions.at(-1);
    if (expression !== undefined && character === "{") {
      expression.braces += 1;
    } else if (expression !== undefined && character === "}") {
      if (expression.braces === 0) {
        openExpressions.pop();
        position = continueLiteral(position + 1, expression.literal);
        continue;
      }
      expression.braces -= 1;
    }
    if (!/\s/.test(character)) {
      regexMayStart = !punctuationBeforeDivision.has(character);
    }
    position += 1;
  }
  return ranges;
}

// commentLinesOf は、コメントのある行の番号（1 から数える）ごとに、その行にかかるコメントの断片
// （// や /* などの印を含む）と、その行がコメントだけでできているかを返す。
export function commentLinesOf(source, language) {
  const lineStarts = [0];
  for (let index = source.indexOf("\n"); index >= 0; index = source.indexOf("\n", index + 1)) {
    lineStarts.push(index + 1);
  }
  // 行の番号ごとの、コメントの断片の [start, end)。
  const spansByLine = new Map();
  let lineIndex = 0;
  for (const range of findCommentRanges(source, language)) {
    while (lineIndex + 1 < lineStarts.length && lineStarts[lineIndex + 1] <= range.start) {
      lineIndex += 1;
    }
    for (let start = range.start; ; start = lineStarts[lineIndex]) {
      const end = Math.min(lineEndOf(source, start), range.end);
      const spans = spansByLine.get(lineIndex + 1) ?? [];
      spans.push({ start, end });
      spansByLine.set(lineIndex + 1, spans);
      if (end >= range.end) {
        break;
      }
      lineIndex += 1;
    }
  }
  const lines = new Map();
  for (const [number, spans] of spansByLine) {
    let code = "";
    let cursor = lineStarts[number - 1];
    for (const { start, end } of spans) {
      code += source.slice(cursor, start);
      cursor = end;
    }
    code += source.slice(cursor, lineEndOf(source, cursor));
    lines.set(number, { pieces: spans.map(({ start, end }) => source.slice(start, end)), commentOnly: code.trim() === "" });
  }
  return lines;
}

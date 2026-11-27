// `git diff -U0 --no-renames --src-prefix=a/ --dst-prefix=b/` の出力から、ファイルごとに消した行と
// 足した行の番号を読む。行の中身は読まない。中身は変更の前と後のファイルを丸ごと読んで調べる。
// コメントは複数行にまたがり、差分の行だけではブロックコメントや文字列の中かが分からないためである。

const hunkHeader = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

// unquotePath は、git が引用符で囲んだパス（"a/x\"y.ts" など）を元に戻す。core.quotePath=false でも、
// 制御文字・" ・\ を含むパスは囲まれる。\ の後の 8 進数は UTF-8 のバイトである。
function unquotePath(quoted) {
  if (!quoted.startsWith('"')) {
    return quoted;
  }
  const escapes = { a: 7, b: 8, f: 12, n: 10, r: 13, t: 9, v: 11 };
  const bytes = [];
  for (let index = 1; index < quoted.length - 1; index += 1) {
    const character = quoted[index];
    if (character !== "\\") {
      bytes.push(...Buffer.from(character));
      continue;
    }
    const octal = quoted.slice(index + 1, index + 4);
    if (/^[0-7]{3}$/.test(octal)) {
      bytes.push(Number.parseInt(octal, 8));
      index += 3;
    } else {
      const escaped = quoted[index + 1];
      bytes.push(escapes[escaped] ?? escaped.charCodeAt(0));
      index += 1;
    }
  }
  return Buffer.from(bytes).toString("utf8");
}

// pathOf は、--- a/… や +++ b/… の行のパスを返す。/dev/null（無いファイル）なら undefined を返す。
function pathOf(headerLine, prefix) {
  const path = unquotePath(headerLine.slice(4));
  return path === "/dev/null" ? undefined : path.slice(prefix.length);
}

function lineNumbers(start, count) {
  return Array.from({ length: count }, (_, offset) => start + offset);
}

// changedLinesOf は、差分の出力から、変更したファイルごとに、パスと、変更の前と後にファイルが
// あるか（oldExists・newExists）と、消した行と足した行の番号（removedLines・addedLines）を返す。
export function changedLinesOf(diff) {
  const files = [];
  let file;
  let oldPath;
  // 読んでいるハンクの、まだ読んでいない消した行と足した行の数。ハンクの中の行は中身が
  // --- や @@ で始まりうるので、数えて読み飛ばす。
  let removedLeft = 0;
  let addedLeft = 0;
  for (const line of diff.split("\n")) {
    if (removedLeft > 0 && line.startsWith("-")) {
      removedLeft -= 1;
    } else if (addedLeft > 0 && line.startsWith("+")) {
      addedLeft -= 1;
    } else if (line.startsWith("diff --git ")) {
      file = undefined;
      oldPath = undefined;
    } else if (line.startsWith("--- ")) {
      oldPath = pathOf(line, "a/");
    } else if (line.startsWith("+++ ")) {
      const newPath = pathOf(line, "b/");
      file = { path: newPath ?? oldPath, oldExists: oldPath !== undefined, newExists: newPath !== undefined, removedLines: [], addedLines: [] };
      files.push(file);
    } else if (file !== undefined && hunkHeader.test(line)) {
      const [, oldStart, oldCount = "1", newStart, newCount = "1"] = line.match(hunkHeader);
      removedLeft = Number(oldCount);
      addedLeft = Number(newCount);
      file.removedLines.push(...lineNumbers(Number(oldStart), removedLeft));
      file.addedLines.push(...lineNumbers(Number(newStart), addedLeft));
    }
  }
  return files;
}

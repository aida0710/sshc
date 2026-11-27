#!/usr/bin/env node
// 変更で足したコメントが日本語かを検査する。sshc のコードのコメントは日本語で書く決まりで、
// いろいろな人や AI ツールが書くので、書く側の注意だけでは守れない。
//
//   node scripts/ci/check-comment-language.mjs [<base> [<head>]]
//
// <head> を渡すと、<base> と分かれた所から <head> までのコミットの差分を見る。CI は
// `HEAD^1 HEAD` を渡す（PR では merge commit の1つ目の親が base で、main の push では1つ前の
// コミット）。<head> を省くと、<base> と分かれた所から今の作業ツリーまでを見る。git に追加して
// いないファイルは見ない。<base> を省くと origin/main と比べる（make test）。
//
// 差分だけを見るので、決まりの前に書いた英語のコメントは、その行を変えるまで落ちない。どの行を
// 英語とみなすか、どの行を通すかは comment-language/englishComments.mjs に書いてある。
// 終了の状態は、違反が無ければ 0、あれば 1、比べる先を読めないなど検査できなければ 2 である。

import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { languageOfPath } from "./comment-language/commentLines.mjs";
import { changedLinesOf } from "./comment-language/diffHunks.mjs";
import { exemptionMarker, findEnglishComments } from "./comment-language/englishComments.mjs";

const defaultBase = "origin/main";

// 見ないファイル。ほかから取り込んだコードと、生成したコードは、ここで書いたコメントではない。
const excludedPaths = [/^third_party\//, /(?:^|\/)node_modules\//, /\.gen\.go$/, /\.generated\.ts$/, /^web\/src\/api\/schema\.d\.ts$/];

// 差分とファイルの中身は、大きな PR でも読み切れるようにする（spawnSync の既定は 1 MiB）。
const gitOutputLimit = 1 << 30;

class UncheckableError extends Error {}

function git(root, args, input) {
  const run = spawnSync("git", args, { cwd: root, input, maxBuffer: gitOutputLimit });
  if (run.error !== undefined) {
    throw run.error;
  }
  return run;
}

function gitText(root, args) {
  const run = git(root, args);
  if (run.status !== 0) {
    throw new UncheckableError(`git ${args.join(" ")} が失敗した: ${run.stderr.toString().trim()}`);
  }
  return run.stdout.toString();
}

function resolveCommit(root, revision) {
  const run = git(root, ["rev-parse", "--verify", "--quiet", "--end-of-options", `${revision}^{commit}`]);
  return run.status === 0 ? run.stdout.toString().trim() : undefined;
}

// readBlobs は、`<コミット>:<パス>` の並びの中身を、git cat-file --batch の1回の実行で読む。
// 無いものは undefined にする。
function readBlobs(root, names) {
  if (names.length === 0) {
    return [];
  }
  const output = git(root, ["cat-file", "--batch"], `${names.join("\n")}\n`).stdout;
  const contents = [];
  let offset = 0;
  for (let index = 0; index < names.length; index += 1) {
    const headerEnd = output.indexOf(10, offset);
    const header = output.subarray(offset, headerEnd).toString();
    offset = headerEnd + 1;
    if (header.endsWith(" missing")) {
      contents.push(undefined);
      continue;
    }
    const size = Number(header.split(" ")[2]);
    contents.push(output.subarray(offset, offset + size).toString("utf8"));
    offset += size + 1;
  }
  return contents;
}

function isChecked(path) {
  return languageOfPath(path) !== undefined && !excludedPaths.some((pattern) => pattern.test(path));
}

// changedFilesBetween は、mergeBase から head（undefined なら作業ツリー）までに変更した、
// 見るファイルの前と後の中身と、消した行と足した行を返す。
function changedFilesBetween(root, mergeBase, head) {
  const diff = gitText(root, [
    "-c",
    "core.quotePath=false",
    "diff",
    "-U0",
    "--no-color",
    "--no-ext-diff",
    "--no-textconv",
    "--no-renames",
    "--no-relative",
    "--src-prefix=a/",
    "--dst-prefix=b/",
    mergeBase,
    ...(head === undefined ? [] : [head]),
  ]);
  const files = changedLinesOf(diff).filter(({ path }) => isChecked(path));
  const oldSources = readBlobs(
    root,
    files.filter(({ oldExists }) => oldExists).map(({ path }) => `${mergeBase}:${path}`),
  );
  const newSources =
    head === undefined
      ? files.filter(({ newExists }) => newExists).map(({ path }) => readFileSync(join(root, path), "utf8"))
      : readBlobs(
          root,
          files.filter(({ newExists }) => newExists).map(({ path }) => `${head}:${path}`),
        );
  return files.map(({ path, oldExists, newExists, removedLines, addedLines }) => ({
    path,
    language: languageOfPath(path),
    oldSource: oldExists ? oldSources.shift() : undefined,
    newSource: newExists ? newSources.shift() : undefined,
    removedLines,
    addedLines,
  }));
}

// 違反の報告に出すコメントの文の長さの上限。行を見分けられれば足りる。
const reportedTextLength = 100;

function report(findings, compared) {
  if (findings.length === 0) {
    console.log(`足したコメントに英語だけの行は無かった（${compared}）`);
    return 0;
  }
  console.error(`足したコメントに英語だけの行が ${findings.length} 行ある（${compared}）。コメントは日本語で書く。`);
  for (const { path, line, text } of findings) {
    const shown = text.length > reportedTextLength ? `${text.slice(0, reportedTextLength)}…` : text;
    console.error(`${path}:${line}: ${shown}`);
  }
  console.error(
    [
      "",
      "かなも漢字も含まず、英単語が3つ以上並ぶコメントの行を、英語の行とみなしている。同じ変更で消した行と",
      "同じ文の行（移しただけのコメント）は通す。移してから書き換えた行は、触った行なので日本語に直す。",
      `英語の仕様の引用のように英語のまま残す理由があるときは、前の行に「// ${exemptionMarker}: 理由」と`,
      "日本語で理由を書く。続くコメントだけの行が通る。",
    ].join("\n"),
  );
  return 1;
}

function main(args) {
  if (args.length > 2) {
    throw new UncheckableError("使い方: node scripts/ci/check-comment-language.mjs [<base> [<head>]]");
  }
  const [base = defaultBase, head] = args;
  const root = gitText(process.cwd(), ["rev-parse", "--show-toplevel"]).trim();
  const baseCommit = resolveCommit(root, base);
  if (baseCommit === undefined) {
    throw new UncheckableError(
      `比べる先の ${base} が無いので、足したコメントを検査できない。git fetch origin で取得するか、比べる先の` +
        "コミットを引数で渡す（make test では COMMITS_BASE=<コミット>）。",
    );
  }
  const headCommit = head === undefined ? undefined : resolveCommit(root, head);
  if (head !== undefined && headCommit === undefined) {
    throw new UncheckableError(`検査する ${head} が無い。`);
  }
  const mergeBase = gitText(root, ["merge-base", baseCommit, headCommit ?? "HEAD"]).trim();
  const changedFiles = changedFilesBetween(root, mergeBase, headCommit);
  const compared = `${changedFiles.length} ファイル、${base} と ${head ?? "作業ツリー"} の差分`;
  return report(findEnglishComments(changedFiles), compared);
}

try {
  process.exitCode = main(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof UncheckableError ? error.message : error);
  process.exitCode = 2;
}

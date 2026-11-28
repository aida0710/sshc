import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { commentLinesOf, languageOfPath } from "../../../scripts/ci/comment-language/commentLines.mjs";
import { repositoryRoot } from "./repositoryRoot";

// gitに追加したソースを、行ごとに、コメントと、コメントを除いた残り（コードと文字列）に分けて読む。コメントの
// 見分け方は、足したコメントの言語の検査（scripts/ci/check-comment-language.mjs）と同じ字句の決まりに従う。

// SourceText は、ソースの1行のうち検査で読む部分と、報告に書く場所（internal/sshclient/auth.go:12 の形）である。
export type SourceText = { text: string; location: string };

// commentsIn は、コメントのある行ごとに、その行にかかるコメントの断片をつないで返す。
export function commentsIn(path: string, source: string): SourceText[] {
  const language = languageOfPath(path);
  if (language === undefined) return [];
  return [...commentLinesOf(source, language)].map(([line, { pieces }]) => ({
    text: pieces.join(" "),
    location: `${path}:${line}`,
  }));
}

// codeOutsideCommentsIn は、どの行も、コメントを除いた残りにして返す。
export function codeOutsideCommentsIn(path: string, source: string): SourceText[] {
  const language = languageOfPath(path);
  if (language === undefined) return [];
  const comments = commentLinesOf(source, language);
  return source.split("\n").map((line, index) => ({
    text: comments.get(index + 1)?.code ?? line,
    location: `${path}:${index + 1}`,
  }));
}

// gitに追加したファイルのパス。消したことをまだgitに伝えていないファイルは読めないので除く。
function trackedFiles(): string[] {
  const listed = execFileSync("git", ["ls-files", "-z"], { cwd: repositoryRoot, encoding: "utf8" });
  return listed.split("\0").filter((path) => path !== "" && existsSync(join(repositoryRoot, path)));
}

function read(path: string): string {
  return readFileSync(join(repositoryRoot, path), "utf8");
}

// 下の2つは、1回のテストの中で何度も呼ばれるので、ファイルを1回だけ読む。
let comments: SourceText[] | undefined;
let engineCode: SourceText[] | undefined;

// repositoryComments は、コメントを読める言語のすべてのファイルの、コメントのある行を返す。
export function repositoryComments(): SourceText[] {
  comments ??= trackedFiles().flatMap((path) => commentsIn(path, read(path)));
  return comments;
}

// sshcエンジンのGoのコード。テストは除く。
const engineSource = /^(?:internal|cmd|mobile)\/.*(?<!_test)\.go$/;
// TypeScriptのコードを生成するコマンド。文字列は生成するコードの一部で、sshcエンジンが書く文ではない。
// 生成したコードのコメントは、生成先（web/src/rules/generated.tsなど）をrepositoryCommentsが読む。
const typeScriptGenerator = /^internal\/(?:validate\/cmd\/rulegen|api\/cmd\/runtimegen)\//;

// engineCodeOutsideComments は、sshcエンジンのGoのコードを、コメントを除いた行にして返す。Goの識別子は
// 英数字なので、ここにある日本語は文字列の中の文である（サーバーのプロンプトと照合する語も入る）。
export function engineCodeOutsideComments(): SourceText[] {
  engineCode ??= trackedFiles()
    .filter((path) => engineSource.test(path) && !typeScriptGenerator.test(path))
    .flatMap((path) => codeOutsideCommentsIn(path, read(path)));
  return engineCode;
}

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, beforeEach, describe, it } from "node:test";
import { fileURLToPath } from "node:url";

const checker = fileURLToPath(new URL("../check-comment-language.mjs", import.meta.url));

// 手元の git の設定（署名、既定のブランチ、利用者の名前）と、外の GIT_* の変数を読まない環境。
// CI の runner には git の利用者の名前が無いので、名前はここで決める。
const gitEnvironment = {
  ...Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith("GIT_"))),
  GIT_CONFIG_GLOBAL: "/dev/null",
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_AUTHOR_NAME: "Dunlin",
  GIT_AUTHOR_EMAIL: "dunlin@example.invalid",
  GIT_COMMITTER_NAME: "Dunlin",
  GIT_COMMITTER_EMAIL: "dunlin@example.invalid",
};

let repository;

function git(...args) {
  const run = spawnSync("git", args, { cwd: repository, env: gitEnvironment, encoding: "utf8" });
  assert.equal(run.status, 0, `git ${args.join(" ")}: ${run.stderr}`);
  return run.stdout.trim();
}

function write(path, source) {
  mkdirSync(dirname(join(repository, path)), { recursive: true });
  writeFileSync(join(repository, path), source);
}

function commit(message) {
  git("add", "--all");
  git("commit", "--quiet", "--allow-empty", "--message", message);
  return git("rev-parse", "HEAD");
}

function check(...args) {
  const run = spawnSync(process.execPath, [checker, ...args], { cwd: repository, env: gitEnvironment, encoding: "utf8" });
  return { status: run.status, stderr: run.stderr };
}

beforeEach(() => {
  repository = mkdtempSync(join(tmpdir(), "sshc-comment-language-"));
  git("init", "--quiet", "--initial-branch=main");
});

afterEach(() => {
  rmSync(repository, { recursive: true, force: true });
});

describe("check-comment-language.mjs", () => {
  it("コミットで英語だけのコメントを足すと、ファイル:行を出して落ちる", () => {
    write("internal/session/open.go", "package session\n\n// Open は接続名のセッションを開く。\nfunc Open() {}\n");
    commit("chore: 最初のコミット");
    write("internal/session/open.go", "package session\n\n// Open は接続名のセッションを開く。\n// It waits for the engine.\nfunc Open() {}\n");
    commit("fix: 英語のコメントを足す");

    const { status, stderr } = check("HEAD^1", "HEAD");
    assert.equal(status, 1, stderr);
    assert.match(stderr, /^internal\/session\/open\.go:4: It waits for the engine\.$/m);
  });

  it("消したファイルから別のファイルへ移しただけのコメントは通る", () => {
    write("web/src/old.ts", "// returns the session of the alias\nexport function old() {}\n");
    commit("chore: 最初のコミット");
    unlinkSync(join(repository, "web/src/old.ts"));
    write("web/src/sessions/open.ts", "export class Sessions {\n  // returns the session of the alias\n  open() {}\n}\n");
    commit("refactor: 移す");

    const { status, stderr } = check("HEAD^1", "HEAD");
    assert.equal(status, 0, stderr);
  });

  it("取り込んだコードと生成したコードの英語のコメントは見ない", () => {
    commit("chore: 最初のコミット");
    const english = "// returns the session of the alias\n";
    write("third_party/xterm/addon.ts", english);
    write("internal/api/models.gen.go", english);
    write("web/src/api/validators.generated.ts", english);
    write("web/src/api/schema.d.ts", english);
    write("scripts/notes.sh", "# returns the session of the alias\n");
    commit("chore: 生成物を足す");

    const { status, stderr } = check("HEAD^1", "HEAD");
    assert.equal(status, 0, stderr);
  });

  it("引数が無ければ、origin/main と分かれた所から作業ツリーまでを見る", () => {
    write("web/src/session.ts", "export const port = 22;\n");
    write("web/src/legacy.ts", "// returns the session of the alias\nexport function legacy() {}\n");
    commit("chore: 最初のコミット");
    // main は分かれたあとで legacy.ts を消した。このブランチは legacy.ts を足していない。
    unlinkSync(join(repository, "web/src/legacy.ts"));
    git("update-ref", "refs/remotes/origin/main", commit("refactor: main で legacy.ts を消す"));
    git("checkout", "--quiet", "-b", "topic", "HEAD^1");
    write("web/src/session.ts", "// the default port of ssh\nexport const port = 22;\n");

    const { status, stderr } = check();
    assert.equal(status, 1, stderr);
    assert.match(stderr, /^web\/src\/session\.ts:1: /m);
    assert.doesNotMatch(stderr, /legacy\.ts/);
  });

  it("比べる先が無いと、取り方を出して検査できなかったとする", () => {
    commit("chore: 最初のコミット");

    const { status, stderr } = check();
    assert.equal(status, 2);
    assert.match(stderr, /origin\/main/);
    assert.match(stderr, /git fetch origin/);
  });
});

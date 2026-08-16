"use strict";

const { symlink, unlink, mkdir, readlink, stat } = require("node:fs/promises");
const { join, dirname } = require("node:path");
const { homedir } = require("node:os");

/**
 * linkPath は、`sshc` を打ったときに走ってほしいものの場所である。
 */
function linkPath() {
  return join(homedir(), ".local", "bin", "sshc");
}

/**
 * relink は、`~/.local/bin/sshc` を束の中の実体へ向け直す。
 *
 * **実体を 1 つにする。** 二つのコピーがあると、コマンドラインと画面が
 * 別の版を走らせることになり、片方だけで直った不具合がもう片方に残る。
 *
 * すでに正しく向いているなら何もしない。**普通のファイルがそこにあるときは
 * 触らない**——`make install` で入れた実体を、断りなく symlink へ変えない。
 * その判断は人のものである。
 *
 * 失敗しても投げない。**リンクが張れないことは、アプリが開けない理由には
 * ならない。**
 */
async function relink(target) {
  // **Windows では、コマンドラインを通すのはインストーラである。**
  //
  // ここで symlink を作ろうとしても、開発者モードか管理者権限が無ければ
  // 失敗するだけであり、仮に作れたとしても `~/.local/bin` は PATH に載って
  // いない。載せるのは NSIS が書く user PATH であって、この外殻ではない。
  if (process.platform === "win32") {
    return {
      changed: false,
      reason: "the installer puts sshc on the path here",
    };
  }
  const path = linkPath();
  try {
    const existing = await readlink(path);
    if (existing === target)
      return { changed: false, reason: "already pointing here" };
  } catch (error) {
    if (error.code === "EINVAL") {
      // symlink ではない。誰かが実体を置いている。
      return { changed: false, reason: "a real file is installed there" };
    }
    if (error.code !== "ENOENT") {
      return { changed: false, reason: String(error.code ?? error.message) };
    }
  }

  try {
    await mkdir(dirname(path), { recursive: true });
    await unlink(path).catch(() => {});
    await symlink(target, path);
    await stat(path);
    return { changed: true, reason: "" };
  } catch (error) {
    return { changed: false, reason: String(error.code ?? error.message) };
  }
}

module.exports = { relink, linkPath };

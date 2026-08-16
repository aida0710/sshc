"use strict";

const promises = require("node:fs/promises");
const { join, dirname } = require("node:path");
const { homedir } = require("node:os");

// managedPath は、外殻が面倒を見る実体の置き場である。
//
// **束の中を指してはならない。** AppImage の中身は一時マウントで、アプリを
// 閉じれば消える——そこへ張ったリンクは、次に `sshc` と打った人には壊れた
// リンクとしてしか見えない。だから束から出して、ここへ写す。
function managedPath() {
  return join(homedir(), ".local", "share", "sshc", "bin", "sshc");
}

// publicPath は、`sshc` と打ったときに走ってほしいものの場所である。
function publicPath() {
  return join(homedir(), ".local", "bin", "sshc");
}

// atomicReplace は、書き終えてディスクに届いたものだけを、その名前にする。
//
// **半分書けたものがその名前を持つ瞬間を作らない。** 実行される実体であれば、
// その瞬間に起動した人は壊れたファイルを実行する。同じディレクトリへ書いて
// から rename するので、rename は境界を跨がない。
async function atomicReplace(fs, path, contents, mode) {
  const temporary = `${path}.${process.pid}.tmp`;
  const handle = await fs.open(temporary, "w", mode);
  try {
    await handle.writeFile(contents);
    await handle.sync();
  } finally {
    await handle.close();
  }
  // open の mode は umask に削られる。実体の権限は約束なので、明示で戻す。
  await fs.chmod(temporary, mode);
  await fs.rename(temporary, path);
  await syncDirectory(fs, dirname(path));
}

// syncDirectory は、rename そのものをディスクへ届ける。できない OS では黙る。
async function syncDirectory(fs, path) {
  try {
    const handle = await fs.open(path, "r");
    try {
      await handle.sync();
    } finally {
      await handle.close();
    }
  } catch {
    // ディレクトリを読み取りで開けない OS がある。届けられないだけで、
    // rename そのものは済んでいる。
  }
}

// sameContents は、写す必要があるかを答える。
//
// **毎回書き直さない。** 起動のたびに実体を置き換えれば、そのとき走っている
// `sshc` の inode が毎回入れ替わる。大きさが違えば読むまでもない。
async function sameContents(fs, source, destination) {
  const [left, right] = await Promise.all([
    fs.stat(source),
    fs.stat(destination).catch(() => null),
  ]);
  if (right === null || left.size !== right.size) return false;
  const [a, b] = await Promise.all([
    fs.readFile(source),
    fs.readFile(destination),
  ]);
  return Buffer.compare(a, b) === 0;
}

/**
 * installManagedCLI は、束の中の CLI を安定した場所へ写し、公開の名前をそこへ
 * 向ける。
 *
 * **公開の名前にあるものが自分の張ったリンクでないなら、触らない。** そこに
 * 何を置くかは利用者の決めたことである——`make install` で入れた実体かも
 * しれないし、別の管理下にあるものかもしれない。断りなく置き換えるのではなく、
 * 何がどこにあるかを名指しして返す。
 */
async function installManagedCLI({
  source,
  managed = managedPath(),
  public: publicTarget = publicPath(),
  fs = promises,
  platform = process.platform,
}) {
  // **Windows では、コマンドラインを通すのはインストーラである。**
  // symlink は開発者モードか管理者権限が要り、`~/.local/bin` は PATH に
  // 載っていない。載せるのは NSIS が書く user PATH であって、この外殻ではない。
  if (platform === "win32") {
    return {
      managed: null,
      copied: false,
      linked: false,
      warning: null,
    };
  }

  await fs.mkdir(dirname(managed), { recursive: true, mode: 0o700 });
  let copied = false;
  if (!(await sameContents(fs, source, managed))) {
    await atomicReplace(fs, managed, await fs.readFile(source), 0o700);
    copied = true;
  }

  const linked = await pointPublicName(fs, publicTarget, managed);
  return { managed, copied, linked: linked.linked, warning: linked.warning };
}

// pointPublicName は、公開の名前を管理下の実体へ向ける。既に他人のものが
// そこに居るなら、向けずにその事実を返す。
async function pointPublicName(fs, path, managed) {
  const occupant = await fs.lstat(path).catch((error) => {
    if (error.code === "ENOENT") return null;
    throw error;
  });

  if (occupant !== null) {
    if (!occupant.isSymbolicLink()) {
      return {
        linked: false,
        warning:
          `${path} already exists and sshc did not create it, so it was left alone. ` +
          `Remove it to let the app manage the sshc command, or keep it and update it yourself.`,
      };
    }
    const existing = await fs.readlink(path).catch(() => null);
    if (existing === managed) return { linked: false, warning: null };
    return {
      linked: false,
      warning:
        `${path} is a link to ${existing ?? "somewhere unreadable"}, not to ${managed}, ` +
        `so it was left alone. Remove it to let the app manage the sshc command.`,
    };
  }

  try {
    await fs.mkdir(dirname(path), { recursive: true });
    await fs.symlink(managed, path);
    return { linked: true, warning: null };
  } catch (error) {
    return {
      linked: false,
      warning: `${path} could not be created (${String(error.code ?? error.message)}).`,
    };
  }
}

module.exports = {
  installManagedCLI,
  managedPath,
  publicPath,
  atomicReplace,
};

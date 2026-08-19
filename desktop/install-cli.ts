import * as promises from "node:fs/promises";
import { join, dirname } from "node:path";
import { homedir } from "node:os";

// FileSystem は、この module が要る分の fs である。
//
// **偽物を受け取るための型ではない。** 今日この引数へ渡されるのは常に本物の
// node:fs/promises であり、テストも一時ディレクトリを作って本物を使う——
// 実体を置き換える・リンクを張る・fsync するという仕事は、偽の fs で確かめても
// 何も確かめたことにならないからである。引数として開いているのは、置き場所を
// テストが差し替えられるようにするためである。
export type FileSystem = typeof promises;

// InstallResult は、端末側の入口を揃えた結果である。
//
// **同じことを二度言わないための仕組みは、ここには無い。** かつてこの module は
// 「前回と同じ報せか」を `.last-notice` というファイルに覚えていた。**それが
// 要ったのは、外殻が起動のたびに黙ってこれを呼んでいたから**である——毎回
// モーダルを出せば、閉じ方を覚えさせるだけだった。いまは利用者がメニューから
// 頼んだときにだけ走るので、二度頼まれたら二度答えるのが正しい。
export type InstallResult = {
  managed: string | null;
  copied: boolean;
  linked: boolean;
  warning: string | null;
  note: string | null;
};

type LinkResult = { linked: boolean; warning: string | null };

// managedPath は、外殻が面倒を見る実体の置き場である。
//
// **束の中を指してはならない。** AppImage の中身は一時マウントで、アプリを
// 閉じれば消える——そこへ張ったリンクは、次に `sshc` と打った人には壊れた
// リンクとしてしか見えない。だから束から出して、ここへ写す。
export function managedPath(): string {
  return join(homedir(), ".local", "share", "sshc", "bin", "sshc");
}

// publicPath は、`sshc` と打ったときに走ってほしいものの場所である。
export function publicPath(): string {
  return join(homedir(), ".local", "bin", "sshc");
}

// atomicReplace は、書き終えてディスクに届いたものだけを、その名前にする。
//
// **半分書けたものがその名前を持つ瞬間を作らない。** 実行される実体であれば、
// その瞬間に起動した人は壊れたファイルを実行する。同じディレクトリへ書いて
// から rename するので、rename は境界を跨がない。
export async function atomicReplace(
  fs: FileSystem,
  path: string,
  contents: string | Uint8Array,
  mode: number,
): Promise<void> {
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
async function syncDirectory(fs: FileSystem, path: string): Promise<void> {
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
async function sameContents(
  fs: FileSystem,
  source: string,
  destination: string,
): Promise<boolean> {
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

// pathNote は、置いた場所が PATH に載っていないなら、そう言う文を返す。
//
// **リンクを張っただけでは `sshc` と打てるようにならない。** macOS の既定の PATH に
// `~/.local/bin` は無く、Debian 系の `~/.profile` がそれを足すのは**ログインの時点で
// そのディレクトリが在ったときだけ**である——初回はこの外殻がいま作ったのだから、
// 在らなかった。`make install` は同じことを note として言っており、こちらだけが
// 黙っていた。
//
// **PATH を自分で確かめない。** GUI から起きたアプリが持つ PATH は launchd や
// デスクトップ環境が渡したものであって、利用者がシェルで見るものではない。
// 確かめられないことを確かめたふりをするより、置いた場所を名指しする。
export function pathNote(publicTarget: string): string {
  return (
    `The sshc command was installed at ${publicTarget}. ` +
    `Make sure ${dirname(publicTarget)} is on your PATH, then open a new terminal to run "sshc".`
  );
}

/**
 * installManagedCLI は、束の中の CLI を安定した場所へ写し、公開の名前をそこへ
 * 向ける。
 *
 * **公開の名前にあるものが自分の張ったリンクでないなら、触らない。** そこに
 * 何を置くかは利用者の決めたことである——`make install` で入れた実体かも
 * しれないし、別の管理下にあるものかもしれない。断りなく置き換えるのではなく、
 * 何がどこにあるかを名指しして返す。
 *
 * note は、公開の名前を**今回作ったとき**にだけ返る。毎回の起動で出すものでは
 * ない——既に PATH を整えた人に、同じ案内を繰り返さない。
 */
export async function installManagedCLI({
  source,
  managed = managedPath(),
  public: publicTarget = publicPath(),
  fs = promises,
  platform = process.platform,
}: {
  source: string;
  managed?: string;
  public?: string;
  fs?: FileSystem;
  platform?: NodeJS.Platform | string;
}): Promise<InstallResult> {
  // **Windows では、コマンドラインを通すのはインストーラである。**
  // symlink は開発者モードか管理者権限が要り、`~/.local/bin` は PATH に
  // 載っていない。載せるのは NSIS が書く user PATH であって、この外殻ではない。
  if (platform === "win32") {
    return { managed: null, copied: false, linked: false, warning: null, note: null };
  }

  await fs.mkdir(dirname(managed), { recursive: true, mode: 0o700 });
  let copied = false;
  if (!(await sameContents(fs, source, managed))) {
    await atomicReplace(fs, managed, await fs.readFile(source), 0o700);
    copied = true;
  }

  const linked = await pointPublicName(fs, publicTarget, managed);
  return {
    managed,
    copied,
    linked: linked.linked,
    warning: linked.warning,
    note: linked.linked ? pathNote(publicTarget) : null,
  };
}

// pointPublicName は、公開の名前を管理下の実体へ向ける。既に他人のものが
// そこに居るなら、向けずにその事実を返す。
async function pointPublicName(
  fs: FileSystem,
  path: string,
  managed: string,
): Promise<LinkResult> {
  const occupant = await fs.lstat(path).catch((error: unknown) => {
    // **ENOENT だけを飲む。** 権限で読めないのは「居ない」ではない——
    // それを居ないと読むと、読めない相手の上に symlink を張りに行く。
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return null;
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
    // **Error とは限らない。** throw されるものに制約は無く、文字列が飛んで
    // くれば code も message も無い——そのまま綴ると理由が「undefined」になる。
    const failure = error as NodeJS.ErrnoException;
    return {
      linked: false,
      warning: `${path} could not be created (${failure.code ?? failure.message ?? String(error)}).`,
    };
  }
}

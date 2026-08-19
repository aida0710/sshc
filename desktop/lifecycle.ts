import type { SpawnOptions } from "node:child_process";

// EngineStatus は、`sshc status` が印字する 1 行の形である。
//
// **cmd/sshc/status.go と対である。** あちらの構造体のタグがこの名前を決める
// ——片方だけを変えると、メニューバーの行が黙って undefined を綴る。
export type EngineStatus = {
  vault: boolean;
  unlocked: boolean;
  sessions: number;
};

/**
 * parseEngineStatus は、`sshc status` の 1 行を読む。
 *
 * **JSON.parse の結果をそのまま信じない。** あれが返すのは any であり、型を
 * 名乗らせただけでは何も確かめたことにならない。読めない行が来たときに
 * `answer.sessions` が undefined のまま流れると、メニューバーは
 * 「コンソール undefined」と綴り、終了の確認は本数を数え損ねる——**どちらも
 * 例外を出さずに間違える。** ここで断れば、呼び出し側の既存の catch へ落ちる。
 */
export function parseEngineStatus(line: string): EngineStatus {
  const parsed: unknown = JSON.parse(line);
  if (typeof parsed !== "object" || parsed === null) {
    throw new Error("status did not answer with an object");
  }
  const answer = parsed as Record<string, unknown>;
  if (
    typeof answer["vault"] !== "boolean" ||
    typeof answer["unlocked"] !== "boolean" ||
    typeof answer["sessions"] !== "number"
  ) {
    throw new Error("status answered with a shape this shell does not know");
  }
  return {
    vault: answer["vault"],
    unlocked: answer["unlocked"],
    sessions: answer["sessions"],
  };
}

// ここから下は、**外殻が子 engine に対して実際に使う分だけ**の姿である。
//
// Electron や Node の本物の型（ChildProcess）をそのまま要求しないのは、
// **テストが本物の子を起こさずに済むようにするため**である——期限で SIGKILL
// する経路や、閉じたチャンネルを無視する子を確かめるのに、本物のプロセスは
// 要らないどころか邪魔になる。狭い型は、この module が本当に依存しているものの
// 一覧でもある。
export type EngineStderr = {
  on(event: "data", listener: (chunk: Buffer | string) => unknown): unknown;
};

export type OwnedEngine = {
  exitCode: number | null;
  signalCode: string | null;
  stdin?: { end(): unknown } | null;
  kill(signal?: "SIGKILL"): unknown;
  once(event: "exit", listener: () => unknown): unknown;
};

// StderrLike は、engine の stderr を流す先である。
export type StderrLike = { write(chunk: Buffer | string): unknown };

// engineSpawnOptions は、所有する子 engine の起こし方である。
//
// **stdin は開いたパイプでなければならない。** それがこのアプリの所有権その
// ものだからである。engine は起動時にそれが生きているチャンネルであることを
// 確かめ、閉じたら自分で片付けて終わる。`ignore` を渡すと /dev/null が届き、
// engine は所有者の居ない起動として断る——窓は開かない。
//
// **stderr も継承ではなくパイプである。** 束にされた GUI アプリには継承できる
// stderr が無い——Windows ではそこに無効なハンドルが渡り、engine の logger が
// 書けなくなる。パイプにするなら読み続けなければならない（64 KiB で埋まった
// 先で止まるのは write を呼んだ engine 自身で、症状は「アプリが黙って固まる」
// になる）ので、読む責任は spawnEngine の側にある。
//
// windowsHide は、子のためのコンソール窓が一瞬出るのを止める。GUI から起こす
// engine に窓は要らない。
export function engineSpawnOptions(): SpawnOptions {
  return { stdio: ["pipe", "pipe", "pipe"], windowsHide: true };
}

// spawnEngine は、Electron が lifetime を所有する子 engine を起こす。
//
// `engine` をここで固定するのは、旧 flag を各 caller が手で渡す余地をなくし、
// parser が公開する owner kind と desktop の起動契約を同じ語に保つためである。
//
// **stderr をこちらの stderr へ流し続ける。** 捨てるとパイプが埋まって engine
// が止まり、素通りさせると `make desktop-run` の端末から engine の理由——ロック
// が取れない、home が解決できない——が消える。
// **返す型を呼び出し側のものに保つ。** 本物の spawn を渡した main は本物の子を
// 受け取り（stdout を読む必要がある）、偽物を渡したテストは偽物を受け取る。
export function spawnEngine<Child extends { stderr?: EngineStderr | null }>(
  spawn: (
    command: string,
    args: readonly string[],
    options: SpawnOptions,
  ) => Child,
  binary: string,
  stderr: StderrLike = process.stderr,
): Child {
  const child = spawn(binary, ["engine"], engineSpawnOptions());
  child.stderr?.on("data", (chunk) => {
    stderr.write(chunk);
  });
  return child;
}

// stopOwnedEngine は、所有権を手放し、子が終わるまで待つ。
//
// **kill ではなく、閉じることが通常の終わり方である。** 閉じれば engine は
// 端末も転送も vault も自分で畳んでからロックを外す。応答しないときのために
// 期限を置くが、そこへ落ちるのは通常経路ではない。
//
// **待つのは、Electron が先に消えないためである。** 親が消えれば子も道連れに
// なるが、それは畳む機会を奪う殺し方であり、開いていた SSH セッションは何も
// 片付けられないまま切れる。
export async function stopOwnedEngine(
  child: OwnedEngine | null,
  timeoutMilliseconds = 5000,
): Promise<void> {
  if (child === null || child.exitCode !== null || child.signalCode !== null)
    return;
  try {
    child.stdin?.end();
  } catch {
    // 既に閉じているだけである。
  }
  await new Promise<void>((done) => {
    const overdue = setTimeout(() => {
      try {
        child.kill("SIGKILL");
      } catch {
        // 既に死んでいるだけである。
      }
      done();
    }, timeoutMilliseconds);
    // **この期限は unref しない。** 呼び出し側は終了する前にこれを待つので、
    // 走らない期限は「閉じたチャンネルを無視する engine が終了を人質に取る」
    // という、期限そのものが防ぐはずの状態になる。子が終われば消えるので、
    // 生きているのは長くても timeoutMilliseconds のあいだだけである。
    child.once("exit", () => {
      clearTimeout(overdue);
      done();
    });
  });
}

// shouldQuitAfterLastWindow は、最後の窓を閉じたときにアプリを終えるかを言う。
//
// **常に否である。OS でも、メニューバーの項目を置けたかどうかでも変わらない。**
// この外殻は engine の寿命そのものであり、窓の寿命はそれより短い——窓を閉じた
// だけで engine を落とせば、解錠済みの vault も、開いている SSH も、動いている
// 端末も一緒に消える。窓を閉じることは、それらを終わらせる意思表示ではない。
//
// **項目を置けなかった環境でも残す。** かつてはそこだけ最後の窓と一緒に終わって
// いた——見えないものを残さないため、という理由だった。だが見える入口は項目
// だけではない。アプリケーションランチャも、裸の `sshc` も、解錠を待つ
// `sshc <接続先>` も、同じ窓を開き直す。届かなくなるわけではない。
export function shouldQuitAfterLastWindow(): boolean {
  return false;
}

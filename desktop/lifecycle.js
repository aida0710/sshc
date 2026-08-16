"use strict";

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
function engineSpawnOptions() {
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
function spawnEngine(spawn, binary, stderr = process.stderr) {
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
async function stopOwnedEngine(child, timeoutMilliseconds = 5000) {
  if (child === null || child.exitCode !== null || child.signalCode !== null)
    return;
  try {
    child.stdin?.end();
  } catch {
    // 既に閉じているだけである。
  }
  await new Promise((done) => {
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
function shouldQuitAfterLastWindow() {
  return false;
}

module.exports = {
  engineSpawnOptions,
  spawnEngine,
  stopOwnedEngine,
  shouldQuitAfterLastWindow,
};

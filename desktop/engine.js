"use strict";

// spawnEngine は、Electron が lifetime を所有する子 engine を起こす。
// `engine` をここで固定するのは、旧 flag を各 caller が手で渡す余地をなくし、
// parser が公開する owner kind と desktop の起動契約を同じ語に保つためである。
//
// **stdin は開いたパイプでなければならない。** それがこのアプリの所有権その
// ものだからである。engine は起動時にそれが生きているチャンネルであることを
// 確かめ、閉じたら自分で片付けて終わる。`ignore` を渡すと /dev/null が届き、
// engine は所有者の居ない起動として断る——窓は開かない。
function spawnEngine(spawn, binary) {
  return spawn(binary, ["engine"], { stdio: ["pipe", "pipe", "inherit"] });
}

// releaseEngine は、所有権を手放す。
//
// **kill ではなく、閉じることが通常の終わり方である。** 閉じれば engine は
// 端末も転送も vault も自分で畳んでからロックを外す。応答しないときのために
// 期限を置くが、そこへ落ちるのは通常経路ではない。
function releaseEngine(child, hardStopMilliseconds = 5000) {
  if (child === null || child.exitCode !== null || child.signalCode !== null)
    return;
  try {
    child.stdin?.end();
  } catch {
    // 既に閉じているだけである。
  }
  const overdue = setTimeout(() => {
    try {
      child.kill("SIGKILL");
    } catch {
      // 既に死んでいるだけである。
    }
  }, hardStopMilliseconds);
  // Electron の終了そのものを、この期限で引き延ばさない。
  overdue.unref?.();
  child.once("exit", () => clearTimeout(overdue));
}

module.exports = { spawnEngine, releaseEngine };

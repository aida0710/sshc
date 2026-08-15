"use strict";

/**
 * installWindowReopener は、既存インスタンスへ届く「もう一度開く」を一つの窓へ
 * 畳み込む。
 *
 * `second-instance` は ready のあとに届くことしか Electron は保証しない。
 * しかし engine の入口を待っているあいだも ready 後なので、main の初期化が
 * 終わる前に届きうる。その要求は捨てず、start() まで一つだけ保留する。
 */
function installWindowReopener({ app, getWindows, createWindow, showFailure }) {
  let started = false;
  let pending = false;
  let opening = null;

  const reopen = () => {
    // 窓の作成中に二度目が来ても、同じ処理を待たせる。どちらも窓が無いと
    // 読んで別々の入口を取り、二枚作る隙を残さない。
    if (opening !== null) return opening;
    const attempt = Promise.resolve()
      .then(async () => {
        const windows = getWindows();
        if (windows.length > 0) {
          const window = windows[0];
          if (window.isMinimized()) window.restore();
          window.show();
          window.focus();
          return;
        }
        await createWindow();
      })
      .catch((error) => showFailure(error))
      .finally(() => {
        if (opening === attempt) opening = null;
      });
    opening = attempt;
    return attempt;
  };

  const request = () => {
    if (!started) {
      pending = true;
      return Promise.resolve();
    }
    return reopen();
  };

  // 二つ目のプロセスは入口の初期化中にも来る。whenReady の長い処理の後まで
  // 登録を遅らせてはいけない。
  app.on("second-instance", request);

  return {
    request,
    start: async () => {
      if (started) return;
      started = true;
      app.on("activate", request);
      if (!pending) return;
      pending = false;
      await reopen();
    },
  };
}

module.exports = { installWindowReopener };

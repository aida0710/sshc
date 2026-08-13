"use strict";

const { app, BrowserWindow, shell, dialog } = require("electron");
const { execFile } = require("node:child_process");
const { join } = require("node:path");
const { existsSync } = require("node:fs");

// engineTimeout は、sshc 側のコマンドひとつに掛ける上限である。
//
// `sshc engine start` は自分の中でも待つ（handoff が書かれるまで）ので、
// ここはその上限より長くしてある。**待ち方を知っているのは Go 側だけ**で
// あり、こちらはそれを追い越さない。
const engineTimeout = 30_000;

/**
 * binary は、束に同梱した sshc の絶対パスを返す。
 *
 * **実体は 1 つである。** 開発中はリポジトリの bin/ を使う——二つのコピーが
 * あると、どちらが走っているのか分からなくなる。
 */
function binary() {
  const bundled = join(process.resourcesPath ?? "", "sshc");
  if (existsSync(bundled)) return bundled;
  return join(__dirname, "..", "bin", "sshc");
}

/**
 * run は sshc をひとつ実行し、標準出力を返す。
 *
 * **argv を直接渡す。** シェルは間に一度も入らない——このアプリケーションが
 * Go 側でずっと守ってきた規則であり、外殻でそれを崩さない。
 */
function run(args) {
  return new Promise((resolve, reject) => {
    execFile(binary(), args, { timeout: engineTimeout }, (error, stdout, stderr) => {
      if (error) {
        reject(new Error(`${args.join(" ")}: ${String(stderr || error.message).trim()}`));
        return;
      }
      resolve(String(stdout).trim());
    });
  });
}

/**
 * entrance は、エンジンが答えている状態にしてから、入口の URL を返す。
 *
 * 二つの呼び出しで済むのは、**どちらの知識も Go 側にあるから**である。
 * 起きているかを確かめ、居なければ起こし、handoff が書かれるまで待つのは
 * `engine start` であり、bootstrap を発行するのは `open --print-url` である。
 */
async function entrance() {
  await run(["engine", "start"]);
  const url = await run(["open", "--print-url"]);
  if (!url.startsWith("http://127.0.0.1:")) {
    throw new Error(`the engine answered with an address this shell will not open: ${url}`);
  }
  return url;
}

/**
 * openWindow は窓をひとつ開き、その URL を読み込む。
 */
function openWindow(url) {
  const window = new BrowserWindow({
    width: 1200,
    height: 800,
    minWidth: 720,
    minHeight: 480,
    title: "sshc",
    backgroundColor: "#111111",
    webPreferences: {
      preload: join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  // **この窓は自分の origin から動かない。** cookie はポートに紐づかないので、
  // 同じ 127.0.0.1 の別のポートへ移動できると、そこがこの session の cookie を
  // 受け取る。外部のリンクは既定のブラウザへ渡す——窓の中では開かない。
  const origin = new URL(url).origin;
  window.webContents.setWindowOpenHandler(({ url: target }) => {
    void shell.openExternal(target);
    return { action: "deny" };
  });
  window.webContents.on("will-navigate", (event, target) => {
    if (new URL(target).origin !== origin) {
      event.preventDefault();
      void shell.openExternal(target);
    }
  });

  void window.loadURL(url);
  return window;
}

/**
 * showFailure は、窓の代わりに理由を出す。**白い画面を見せない。**
 */
function showFailure(error) {
  dialog.showErrorBox("sshc could not start", String(error.message ?? error));
}

app.whenReady().then(async () => {
  try {
    openWindow(await entrance());
  } catch (error) {
    showFailure(error);
    app.quit();
    return;
  }

  app.on("activate", async () => {
    if (BrowserWindow.getAllWindows().length > 0) return;
    try {
      openWindow(await entrance());
    } catch (error) {
      showFailure(error);
    }
  });
});

// macOS でも窓を閉じたら終わる。
//
// **窓が無いのに動き続ける外殻には意味が無い。** エンジンを残すかどうかは
// 別の話であり、それは設定が決める——下の before-quit がそれを Go 側へ尋ねる。
app.on("window-all-closed", () => app.quit());

let quitting = false;
app.on("before-quit", (event) => {
  if (quitting) return;
  event.preventDefault();
  quitting = true;
  // **止めるかどうかを決めるのは設定である。** ここではそれを読まない——
  // metadata の形を知る場所を二つにしないため、判断は Go 側が持つ。
  run(["engine", "quit"])
    .catch(() => {})
    .finally(() => app.quit());
});

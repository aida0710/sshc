"use strict";

const { app, BrowserWindow, Menu, nativeImage, shell, dialog } = require("electron");
const { execFile } = require("node:child_process");
const { join } = require("node:path");
const { existsSync } = require("node:fs");
const { relink } = require("./link");

// 名乗る名前をここで決める。
//
// **packaged された束は package.json の productName を使うが、開発中は違う。**
// `npm start` が起こしているのは Electron.app そのものなので、放っておくと
// 通知も userData も「Electron」の名前で作られる。ready より前に呼ぶ必要が
// あるのは、userData の場所がその時点で決まるからである。
//
// **macOS のメニューバーの一番左だけは、これでは変わらない。** あれは走って
// いる束の Info.plist から来るので、開発中は Electron のままである。
// `make desktop-dist` で作った束は sshc と名乗る。
app.setName("sshc");

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
 * `engine start` であり、bootstrap を発行するのは `open` である。
 */
async function entrance() {
  // **実体を 1 つにする。** 二つのコピーがあると、コマンドラインと画面が
  // 別の版を走らせることになる。失敗しても続ける——リンクが張れないことは、
  // アプリが開けない理由にはならない。
  await relink(binary());

  await run(["engine", "start"]);
  const url = await run(["open"]);
  if (!url.startsWith("http://127.0.0.1:")) {
    throw new Error(`the engine answered with an address this shell will not open: ${url}`);
  }
  return url;
}

/**
 * openWindow はウィンドウをひとつ開き、その URL を読み込む。
 */
function openWindow(url) {
  const window = new BrowserWindow({
    width: 1200,
    height: 800,
    minWidth: 720,
    minHeight: 480,
    title: "sshc",
    backgroundColor: "#111111",
    // Linux はウィンドウ自身が図を運ぶ。macOS は束から読むので無視される。
    ...(icon() === null ? {} : { icon: icon() }),
    webPreferences: {
      preload: join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  // **このウィンドウは自分の origin から動かない。** cookie はポートに紐づかないので、
  // 同じ 127.0.0.1 の別のポートへ移動できると、そこがこの session の cookie を
  // 受け取る。外部のリンクは既定のブラウザへ渡す——ウィンドウの中では開かない。
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
 * showFailure は、ウィンドウの代わりに理由を出す。**白い画面を見せない。**
 */
function showFailure(error) {
  dialog.showErrorBox("sshc could not start", String(error.message ?? error));
}

/**
 * icon は、束に入っている図を返す。無ければ null。
 *
 * **packaged された束では要らない。** あちらのアイコンは OS が束から読む。
 * ここが効くのは開発中だけであり、そこでは走っているのが Electron.app
 * そのものなので、放っておくと Electron の図が出る。
 */
function icon() {
  const path = join(__dirname, "build", "icon.png");
  if (!existsSync(path)) return null;
  const image = nativeImage.createFromPath(path);
  return image.isEmpty() ? null : image;
}

/**
 * installMenu は、この外殻のメニューを置く。
 *
 * **役割（role）だけで組む。** コピーも貼り付けも、OS が既に知っている操作で
 * あり、こちらが実装するものは何も無い——役割で書くと、割り当てられている
 * キーも各国語の名前も OS が付ける。
 *
 * 端末の中のコピーは別である。**xterm の選択はブラウザの選択ではない**ので、
 * あちらは画面側が自分で写す。ここに置くのは、それ以外のすべての場所——
 * 入力欄、鍵の指紋、エラーの文言——のための道である。
 */
function installMenu() {
  const application = process.platform === "darwin"
    ? [{ role: "appMenu" }]
    : [];
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    ...application,
    { role: "editMenu" },
    { role: "viewMenu" },
    { role: "windowMenu" },
  ]));
}

app.whenReady().then(async () => {
  installMenu();
  const image = icon();
  if (image !== null && app.dock !== undefined) app.dock.setIcon(image);
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

// macOS でもウィンドウを閉じたら終わる。
//
// **ウィンドウが無いのに動き続ける外殻には意味が無い。** エンジンを残すかどうかは
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

// Ctrl-C でも「終わる」を通す。
//
// **端末から起こした外殻は SIGINT を受けてその場で死に、before-quit は走らない。**
// エンジンは親を持たないデーモンなので、そのまま生き残る——しかも次に起きた
// エンジンが handoff を上書きした瞬間、それは誰からも見えなくなる。止める術も、
// 次の起動で見つける術も無い 1 台が、開発の一巡ごとに増えていた。
//
// 二度目の合図は待たない。一度目の後始末が返らないなら、待っているものは
// もう答えないものである。
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    if (quitting) process.exit(1);
    app.quit();
  });
}

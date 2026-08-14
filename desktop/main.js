"use strict";

const { app, BrowserWindow, Menu, nativeImage, shell, dialog } = require("electron");
const { execFile, spawn } = require("node:child_process");
const { join } = require("node:path");
const { existsSync } = require("node:fs");
const { relink } = require("./link");
const { parseEntrance } = require("./entrance");
const { installTray } = require("./tray");

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

// **エンジンが 2 台になる道を、ここで塞ぐ。** 起こし手がひとつなら、
// 名簿もロックファイルも要らない。
if (!app.requestSingleInstanceLock()) app.exit(0);

// engineTimeout は、run() が execFile ひとつに掛ける上限である。
//
// 値そのものは「フリーズしたまま待ち続けない」ための保険でしかない。
const engineTimeout = 30_000;

// hidden は、窓を作らずに上がるべきかを言う。
//
// **端末から起こされたときの姿である。** `launchApp()`（Go 側）が
// `open -g -b <bundleID> --args --hidden` で起こすので、メニューバーの
// 項目だけが出て、画面を奪わない。
const hidden = process.argv.includes("--hidden");

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
 * status は、エンジンがいまどうなっているかを尋ねる。
 *
 * **handoff の秘密を持つのは Go 側だけ**なので、外殻は `sshc status` を
 * 叩いて尋ねる。自分では handoff を読まない。メニューバーと
 * before-quit の本数、両方がここを通る。
 */
function status() {
  return run(["status"]).then((line) => JSON.parse(line));
}

// engine は、このアプリが起こしたエンジンひとつである。
//
// **detach しない。** 親と一緒に死ぬことが、このアプリケーションが
// 「終了すれば全部止まる」と言えることの実装そのものである。
let engine = null;

// tray は、メニューバーに置いた項目ひとつである。
let tray = null;

/**
 * entrance は、エンジンを起こし、入口の URL を返す。
 */
function entrance() {
  return new Promise((resolve, reject) => {
    // **実体を 1 つにする。** 失敗しても続ける——リンクが張れないことは、
    // アプリが開けない理由にはならない。
    relink(binary()).catch(() => {});

    const child = spawn(binary(), [], { stdio: ["ignore", "pipe", "pipe"] });
    engine = child;

    let buffered = "";
    const timer = setTimeout(
      () => reject(new Error("the engine printed no entrance within 20s")),
      20_000,
    );
    const settle = (error, url) => {
      clearTimeout(timer);
      if (error) reject(error);
      else resolve(url);
    };

    child.stdout.on("data", (chunk) => {
      buffered += String(chunk);
      const url = parseEntrance(buffered);
      if (url !== null) settle(null, url);
    });
    child.on("error", (error) => settle(error));
    child.on("exit", (code) => settle(new Error(`the engine exited with ${code}`)));
  });
}

/**
 * openWindow はウィンドウをひとつ開き、その URL を読み込む。
 */
function openWindow(url) {
  // **窓を開くたびに Dock へ戻す。** 窓が無いあいだは隠している
  // （window-all-closed）——ここで戻さないと、開き直しても Dock に
  // 出てこない。
  if (app.dock !== undefined) app.dock.show();

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
    // **entrance() を通るのは、この 1 回だけである。** エンジンを spawn
    // するのはここだけでよい——以降の開き直しは、走っているエンジンに
    // 新しい入口を出させる run(["open"]) を使う。ここで entrance() を
    // 使い回すと、窓を閉じて開くたびにエンジンがもう 1 台増える。
    //
    // **--hidden のときは窓を作らない。** エンジンは起こす——メニューバーの
    // 項目が「動いている」ことの証拠であり、端末はここが上げたエンジンに
    // 繋ぎに行く。
    const url = await entrance();
    if (!hidden) openWindow(url);
  } catch (error) {
    showFailure(error);
    app.quit();
    return;
  }

  /**
   * reopen は、既にある窓を出すか、無ければ run(["open"]) で新しい窓を開く。
   * activate と Tray の「ウィンドウを開く」の両方がここを通る。
   */
  const reopen = async () => {
    const windows = BrowserWindow.getAllWindows();
    if (windows.length > 0) {
      windows[0].show();
      return;
    }
    try {
      openWindow(await run(["open"]));
    } catch (error) {
      showFailure(error);
    }
  };

  tray = installTray({
    onOpen: reopen,
    onQuit: () => app.quit(),
    status,
  });

  app.on("activate", reopen);
});

// **窓を閉じてもアプリは残る。** メニューバーの項目がその意味である
// ——ウィンドウが無いのに動き続ける外殻に意味を与えたのは、あの項目である。
app.on("window-all-closed", () => {
  if (app.dock !== undefined) app.dock.hide();
});

/**
 * liveSessions は、生きているコンソールの本数を返す。エンジンに尋ねられ
 * なければ 0 を返す——**終了を止める理由にはできない**。本数が分からない
 * ことは、開いているコンソールが無いことの証明にはならないが、それを理由に
 * 終了できなくしてよいわけでもない。
 */
async function liveSessions() {
  try {
    return (await status()).sessions;
  } catch {
    return 0;
  }
}

let quitting = false;
// **確認の最中かどうかの印。** liveSessions() やダイアログを待っている間に
// before-quit がもう一度届く道がある——メニューから終了を選んだ直後に
// Ctrl-C を叩く、など。quitting は確認が終わってからでないと立たないので、
// 待っている間の二度目をこれだけでは弾けない。ここを await の手前で立てる
// ことで、二枚目のダイアログを防ぐ。
let confirmingQuit = false;
app.on("before-quit", async (event) => {
  if (quitting) return;
  event.preventDefault();
  if (confirmingQuit) return;
  confirmingQuit = true;
  // **SSH のセッションは閉じたら戻らない。** 本数を出して一度だけ問う。
  const open = await liveSessions();
  if (open > 0) {
    const answer = await dialog.showMessageBox({
      type: "question",
      buttons: ["終了", "やめる"],
      defaultId: 1,
      cancelId: 1,
      message: `開いているコンソールが ${open} 本あります。終了しますか。`,
    });
    if (answer.response !== 0) {
      confirmingQuit = false;
      return;
    }
  }
  quitting = true;
  if (engine !== null) engine.kill();
  app.quit();
});

// Ctrl-C でも「終わる」を通す。
//
// **既定では SIGINT を受けてその場で死に、before-quit は走らない。** それでも
// ここを通すのは、エンジンを確実に畳んでからアプリを終えるためである
// （エンジンは detach していない子なので、素通りしても道連れにはなるが、
// ウィンドウを畳む機会は失う）。
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => app.quit());
}

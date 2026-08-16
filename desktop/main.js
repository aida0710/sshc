"use strict";

const {
  app,
  BrowserWindow,
  Menu,
  nativeImage,
  shell,
  dialog,
} = require("electron");
const { execFile, spawn } = require("node:child_process");
const { join } = require("node:path");
const { existsSync } = require("node:fs");
const { installManagedCLI } = require("./install-cli");
const { recordLinuxLauncher } = require("./launcher");
const { parseEntrance } = require("./entrance");
const {
  spawnEngine,
  stopOwnedEngine,
  shouldQuitAfterLastWindow,
} = require("./lifecycle");
const { installTray } = require("./tray");
const { installWindowReopener } = require("./reopen");

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

// **外殻がエンジンを 2 台起こす道を、ここで塞ぐ。** Go 側の flock は、
// 端末から裸の sshc が同時に起こされる別の道を塞ぐために残す。
if (!app.requestSingleInstanceLock()) app.exit(0);

// **二度目の起動を、先にいる窓へ渡す。** `second-instance` は ready のあとに
// 届くが、engine の入口を待っている最中にも届きうるので、whenReady の処理が
// 終わるまで登録を遅らせない。
const windowReopener = installWindowReopener({
  app,
  getWindows: () => BrowserWindow.getAllWindows(),
  createWindow: async () => openWindow(await run(["open"])),
  showFailure,
});

// engineTimeout は、run() が execFile ひとつに掛ける上限である。
//
// 値そのものは「フリーズしたまま待ち続けない」ための保険でしかない。
const engineTimeout = 30_000;

// engineBusy は、「エンジンの持ち主が別に居た」とエンジンが答える終了コードで
// ある。**cmd/sshc/main.go の engineBusyExit と対である。**
//
// この番号があるのは、「自分が起こしたエンジンが死んだ」と「別の持ち主が既に
// 居た」を、外殻が区別できなければならないからである。前者はアプリを終える
// 理由だが、後者は理由を出す理由である。
const engineBusy = 3;

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
    execFile(
      binary(),
      args,
      { timeout: engineTimeout },
      (error, stdout, stderr) => {
        if (error) {
          reject(
            new Error(
              `${args.join(" ")}: ${String(stderr || error.message).trim()}`,
            ),
          );
          return;
        }
        resolve(String(stdout).trim());
      },
    );
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
    // **engine を渡す。** 他人のエンジンの入口を受け取ると、窓は開くのに
    // Cmd+Q でそのエンジンが残る——このアプリが「終了すれば全部止まる」と
    // 言えなくなる。あちらは入口を出さずに engineBusy で終わる。
    const child = spawnEngine(spawn, binary());
    engine = child;

    let buffered = "";
    // **決着したかどうかの印。** Promise は一度しか決着しないので、resolve の
    // あとの reject は何も起こさない——エンジンが落ちても外殻が気づかないのは
    // それが理由だった。ここを見て、決着後は別の道（アプリを終える）へ行く。
    let settled = false;
    const timer = setTimeout(
      () => settle(new Error("the engine printed no entrance within 20s")),
      20_000,
    );
    const settle = (error, url) => {
      settled = true;
      clearTimeout(timer);
      if (error) reject(error);
      else resolve(url);
    };

    child.stdout.on("data", (chunk) => {
      // **入口の 1 行を読んだあとは溜めない。** 溜め続ければ、エンジンが
      // 標準出力へ書くだけ buffered が伸びる。listener は残すので、パイプは
      // 読まれ続けて埋まらない。
      if (settled) return;
      buffered += String(chunk);
      const url = parseEntrance(buffered);
      if (url !== null) settle(null, url);
    });
    child.on("error", (error) => settle(error));
    child.on("exit", (code) => {
      // **settled を先に見る。** 今日、engineBusy（3）を出す場所はツリー全体で
      // 1 箇所だけで、それは入口を印字するより前にあるので、settled が真の
      // ときに code === engineBusy になることは今日は起きない——判定の
      // 順番を変えても今日の挙動は変わらない。それでも settled を先に置くのは、
      // 将来「入口を印字したあとに 3 で終わる」経路が増えたときに備えてである。
      // 判定の順が逆だと、その経路はここで engineBusy 扱いになり、既に上がって
      // 落ちたエンジンの死をこのアプリが黙って見逃す——この枝で直したばかりの
      // 形そのものに戻ってしまう。
      //
      // **アプリはエンジンの寿命そのものである。** 上がったあとに落ちたなら、
      // 窓もメニューバーの項目も、もう何も配れない——残しておく意味がない。
      if (settled) {
        showFailure(
          new Error(
            `the engine exited with ${code}; sshc cannot serve anything without it`,
          ),
        );
        quitting = true;
        app.quit();
        return;
      }
      // **持ち主が別に居たなら、断る。** この番号で終わった子は入口を 1 行も
      // 書いていないので、ここは必ず settled より先に来る。
      //
      // 相乗りしない理由は、相乗りできないからである。engine に入っているのは
      // 他人の死んだ子なので、終了時の engine.kill() は何も殺さない——窓は
      // 開くが、Cmd+Q でエンジンだけが残る。**持ち主はアプリひとつ**という
      // 決めごとを、窓を開くために曲げる方が高くつく。
      if (code === engineBusy) {
        settle(
          new Error(
            "端末で動いている sshc がエンジンです。" +
              "その端末で Ctrl-C を押して終わらせてから、もう一度開いてください。",
          ),
        );
        return;
      }
      settle(new Error(`the engine exited with ${code}`));
    });
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
  const application =
    process.platform === "darwin" ? [{ role: "appMenu" }] : [];
  Menu.setApplicationMenu(
    Menu.buildFromTemplate([
      ...application,
      { role: "editMenu" },
      { role: "viewMenu" },
      { role: "windowMenu" },
    ]),
  );
}

/**
 * settleInstallation は、外殻が上がるたびに端末側の入口を揃える。
 *
 * **アプリが開ける理由にも、開けない理由にもしない。** 失敗しても窓は出す
 * ——`sshc` と打てないことと、画面が見られないことは別の話である。ただし
 * 黙りもしない: 公開の名前を他人が持っていたなら、その事実を出す。
 */
async function settleInstallation() {
  try {
    const { warning } = await installManagedCLI({ source: binary() });
    if (warning !== null) {
      dialog.showMessageBox({
        type: "warning",
        message: "sshc could not install the command line",
        detail: warning,
      });
    }
  } catch {
    // 写せないことは、アプリが開けない理由にはならない。
  }
  try {
    await recordLinuxLauncher({ packaged: app.isPackaged });
  } catch {
    // 場所を書き残せないだけである。窓もエンジンも動く。
  }
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
    // **ここで確認は出さない。** 上がらなかったのだから、このアプリが抱えて
    // いるコンソールは 1 本も無い。それでも before-quit を素通りさせると、
    // 端末で動いている別のエンジンの本数を数えて「終了しますか」と訊き、
    // 「やめる」と答えた人には窓もエンジンも無い外殻だけが残る。
    //
    // **quitting を立てる前に engine.kill() を呼ぶ。** before-quit は
    // quitting が真なら早期 return するので、先に立ててしまうと自分の
    // engine.kill() まで一緒に飛ぶ。20 秒 timeout で決着する経路（子は
    // 生きているのに入口が来ない）では、これを飛ばすと子が engine.lock を
    // 握ったまま孤児になり、見張りが畳むまで残る——その窓に開き直すと
    // 「端末で動いている sshc がエンジンです」という誤った理由が出る。
    void stopOwnedEngine(engine);
    quitting = true;
    app.quit();
    return;
  }

  // **置けないことがある。** appindicator の無い Linux では `new Tray` が
  // 投げる。ここで投げさせると、この先の `activate` の登録まで巻き添えで
  // 止まる——窓もメニューバーの項目も無く、kill でしか終われないプロセスが
  // 残る。**この枝が消しに来たものそのものである。**
  try {
    tray = installTray({
      onOpen: windowReopener.request,
      onQuit: () => app.quit(),
      status,
    });
  } catch {
    tray = null;
  }

  await windowReopener.start();
  // 窓を先に出す。**端末側の入口を揃えるのは、画面を待たせてよい仕事ではない。**
  await settleInstallation();
});

// **窓を閉じてもアプリは残る。** この外殻はエンジンの寿命そのものであり、
// 窓の寿命はそれより短い。判断は lifecycle が持つ——メニューバーの項目を
// 置けたかどうかでも、どの OS かでも変わらない。
app.on("window-all-closed", () => {
  if (shouldQuitAfterLastWindow()) {
    app.quit();
    return;
  }
  if (app.dock !== undefined) app.dock.hide();
});

// sessionCountTimeout は、終了の確認のために本数を尋ねる上限である。
//
// **これが無いと、答えないエンジンが終了そのものを人質に取る。** 尋ねるのは
// execFile で起こす別プロセスであり、その既定の上限は 30 秒ある。before-quit は
// その間 preventDefault したままなので、窓も閉じず、エンジンも解放されない。
const sessionCountTimeout = 2000;

/**
 * liveSessions は、生きているコンソールの本数を返す。エンジンに尋ねられ
 * なければ 0 を返す——**終了を止める理由にはできない**。本数が分からない
 * ことは、開いているコンソールが無いことの証明にはならないが、それを理由に
 * 終了できなくしてよいわけでもない。答えが遅すぎることも同じである。
 */
async function liveSessions(timeoutMilliseconds = sessionCountTimeout) {
  let timer = null;
  const overdue = new Promise((resolve) => {
    timer = setTimeout(() => resolve(0), timeoutMilliseconds);
    timer.unref?.();
  });
  try {
    return await Promise.race([
      status().then((answer) => answer.sessions),
      overdue,
    ]);
  } catch {
    return 0;
  } finally {
    if (timer !== null) clearTimeout(timer);
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
  // **子が畳み終わるまで待つ。** 待たずに終えれば親と一緒に道連れになるが、
  // それは畳む機会を奪う殺し方であり、開いていた SSH は何も片付けられない
  // まま切れる。応答しないときのために stopOwnedEngine が期限を持つ。
  await stopOwnedEngine(engine);
  app.quit();
});

// **解放を確認の経路だけに預けない。** before-quit は preventDefault で
// 止まり、ダイアログや遅い問い合わせを待つ。実際に終わる経路でももう一度
// 手放す——stopOwnedEngine は二度呼んでよい。ここでは待てないので、閉じる
// ところまでを行う。
app.on("will-quit", () => {
  void stopOwnedEngine(engine);
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

"use strict";

const { Menu, Tray, nativeImage } = require("electron");
const { join } = require("node:path");

/**
 * installTray は、メニューバーに項目をひとつ置く。
 *
 * **これは飾りではない。** このアプリはウィンドウを閉じても終わらないので、
 * 動いていることの証拠がどこにも無くなる。転送 1 本について「開いていることが
 * 見えないまま開かない」と言っている以上、エンジン自身がその規則の外に居ては
 * ならない。
 */
function installTray({ onOpen, onQuit, status }) {
  const image = nativeImage.createFromPath(join(__dirname, "build", "trayTemplate.png"));
  image.setTemplateImage(true);
  const tray = new Tray(image);
  tray.setToolTip("sshc");

  // **開くたびに数え直す。** 貼りっぱなしのメニューは、閉じたはずの
  // コンソールをいつまでも数えて見せる。
  const show = async () => {
    let line = "sshc";
    try {
      const answer = await status();
      line = `${answer.unlocked ? "解錠中" : "施錠中"} · コンソール ${answer.sessions}`;
    } catch {
      line = "エンジンに繋がりません";
    }
    tray.popUpContextMenu(Menu.buildFromTemplate([
      { label: line, enabled: false },
      { type: "separator" },
      { label: "ウィンドウを開く", click: onOpen },
      { label: "sshc を終了", click: onQuit },
    ]));
  };

  tray.on("click", show);
  tray.on("right-click", show);
  return tray;
}

module.exports = { installTray };

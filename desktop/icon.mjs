// icon.mjs は build/icon.svg と build/tray.svg から各 PNG を焼く。
//
// **正本は SVG ひとつずつである。** PNG を手で置くと、次に誰かが図を直したときに
// 古い PNG が残る——どちらが本当かを知る手段が無くなる。ここを通せば、
// 直すのは常に SVG であり、束に入るのはそれを焼いたものになる。
//
// 焼くのに使うのは、この repo が既に持っている Chromium である（Playwright)。
// **変換のためだけに依存を増やさない。** ImageMagick も rsvg も librsvg も、
// この数枚のためには重い。
//
// electron-builder は 1024×1024 の PNG ひとつから macOS の .icns と Linux の
// 各寸法を作る。**寸法ごとの PNG をここで並べない**——並べれば、それも
// 手で揃え続けるものになる。メニューバーの図だけは例外で、macOS 自身が
// 16px と 32px（Retina）の 2 枚を要求する。

import { readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));

// Playwright は web の依存である。**外殻に同じものを入れ直さない**——
// 焼くのは開発のときだけであり、束には入らない。
const require = createRequire(import.meta.url);
const { chromium } = require(
  require.resolve("playwright", { paths: [join(here, "..", "web", "node_modules")] }),
);

/**
 * bake は 1 枚の SVG を、指定した正方形の寸法の PNG として書く。
 *
 * **browser は呼び出し元と共有する。** 起動そのものが重く、焼くたびに
 * 起こし直す理由が無い。
 */
async function bake(browser, source, target, size) {
  const svg = await readFile(source, "utf8");
  const page = await browser.newPage({
    viewport: { width: size, height: size },
    deviceScaleFactor: 1,
  });
  try {
    // 角の外は透明でなければならない。**背景を敷くと角が四角くなる**ので、
    // macOS の丸みが出ない。
    await page.setContent(
      `<style>html,body{margin:0;padding:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`,
    );
    await writeFile(target, await page.screenshot({ omitBackground: true }));
  } finally {
    await page.close();
  }
  console.log(`wrote ${target} (${size}x${size})`);
}

const browser = await chromium.launch();
try {
  // 1024 は electron-builder が求める最大の寸法である。ここから下は縮小で
  // 作れるが、上は作れない。
  await bake(browser, join(here, "build", "icon.svg"), join(here, "build", "icon.png"), 1024);

  // メニューバーの図は単色・輪郭だけの別の図である（build/tray.svg）。
  // **macOS は Template で終わる名前を見て、明暗に合わせて色を反転する**
  // ので、この名前は飾りではない。16px と 32px（Retina）の 2 枚を焼く。
  const trayPath = join(here, "build", "tray.svg");
  await bake(browser, trayPath, join(here, "build", "trayTemplate.png"), 16);
  await bake(browser, trayPath, join(here, "build", "trayTemplate@2x.png"), 32);
} finally {
  await browser.close();
}

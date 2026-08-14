// icon.mjs は build/icon.svg から build/icon.png を焼く。
//
// **正本は SVG ひとつである。** PNG を手で置くと、次に誰かが図を直したときに
// 古い PNG が残る——どちらが本当かを知る手段が無くなる。ここを通せば、
// 直すのは常に SVG であり、束に入るのはそれを焼いたものになる。
//
// 焼くのに使うのは、この repo が既に持っている Chromium である（Playwright)。
// **変換のためだけに依存を増やさない。** ImageMagick も rsvg も librsvg も、
// この 1 枚のためには重い。
//
// electron-builder は 1024×1024 の PNG ひとつから macOS の .icns と Linux の
// 各寸法を作る。**寸法ごとの PNG をここで並べない**——並べれば、それも
// 手で揃え続けるものになる。

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
const source = join(here, "build", "icon.svg");
const target = join(here, "build", "icon.png");

// 1024 は electron-builder が求める最大の寸法である。ここから下は縮小で
// 作れるが、上は作れない。
const size = 1024;

const svg = await readFile(source, "utf8");

const browser = await chromium.launch();
try {
  const page = await browser.newPage({
    viewport: { width: size, height: size },
    deviceScaleFactor: 1,
  });
  // 角の外は透明でなければならない。**背景を敷くと角が四角くなる**ので、
  // macOS の丸みが出ない。
  await page.setContent(
    `<style>html,body{margin:0;padding:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`,
  );
  await writeFile(target, await page.screenshot({ omitBackground: true }));
} finally {
  await browser.close();
}

console.log(`wrote ${target} (${size}x${size})`);

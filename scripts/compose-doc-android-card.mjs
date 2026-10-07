// Lays two phone screenshots side by side on a 16:9 canvas, because the docs home cards crop
// every image to 16:9 and a single portrait screenshot would lose most of the screen.
//
//   node scripts/compose-doc-android-card.mjs <visual-dir> <output.png>
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";

const require = createRequire(new URL("../web/package.json", import.meta.url));
const { chromium } = require("playwright");

const phoneScreenshots = ["sshc-home-quick-access-mobile.png", "sshc-connections-management-mobile-list.png"];
const [visualDirectory, outputPath] = process.argv.slice(2);
if (visualDirectory === undefined || outputPath === undefined) {
  throw new Error("usage: compose-doc-android-card.mjs <visual-dir> <output.png>");
}
const sources = await Promise.all(phoneScreenshots.map(async (name) =>
  `data:image/png;base64,${(await readFile(`${visualDirectory}/${name}`)).toString("base64")}`));
const browser = await chromium.launch();
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  await page.setContent(`<!doctype html><style>
    html, body { margin: 0; height: 100%; background: #15171a; }
    body { display: flex; align-items: center; justify-content: center; gap: 72px; }
    img { height: 640px; border: 2px solid #3b4045; border-radius: 24px; }
  </style>${sources.map((source) => `<img src="${source}" alt="">`).join("")}`);
  await page.waitForFunction(() => [...document.images].every((image) => image.complete && image.naturalWidth > 0));
  await page.screenshot({ path: outputPath });
} finally {
  await browser.close();
}

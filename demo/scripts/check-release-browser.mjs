import assert from "node:assert/strict";
import { readFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { chromium } from "playwright";

const demoURL = process.env.SSHC_DEMO_TEST_URL ?? "http://127.0.0.1:4178/index.html";
const artifactDirectory = resolve(process.env.SSHC_DEMO_ARTIFACTS ?? "../artifacts/demo/release-check");
const releaseDirectory = resolve(process.argv[2]);
const localConfiguration = JSON.parse(await readFile(new URL("../dist/config.json", import.meta.url), "utf8"));
const manifest = JSON.parse(await readFile(resolve(releaseDirectory, `sshc-demo-${localConfiguration.version}.json`), "utf8"));
const archive = await readFile(resolve(releaseDirectory, manifest.fileName));
const origin = new URL(demoURL).origin;
await mkdir(artifactDirectory, { recursive: true });
const browser = await chromium.launch({ headless: true });
try {
  // The demo follows the browser language; these checks read the Japanese screen.
  const context = await browser.newContext({ viewport: { width: 1440, height: 980 }, locale: "ja-JP" });
  let archiveRequests = 0;
  await context.route(`${origin}/config.json`, async (route) => {
    const response = await route.fetch();
    const configuration = await response.json();
    await route.fulfill({ json: { ...configuration, version: "dev", releaseProxyURL: `${origin}/` } });
  });
  await context.route(`${origin}/latest.json`, route => route.fulfill({ json: {
    ...manifest, archiveURL: `${origin}/archives/${manifest.version}.tar.gz`,
  } }));
  await context.route(`${origin}/archives/${manifest.version}.tar.gz`, (route) => {
    archiveRequests++;
    return route.fulfill({ body: archive, contentType: "application/octet-stream" });
  });
  const page = await context.newPage();
  page.setDefaultTimeout(60_000);
  await page.goto(demoURL);
  await page.locator("#release-status").filter({ hasText: manifest.version }).waitFor();
  assert.equal(archiveRequests, 0);
  assert.equal(await page.evaluate(() => Boolean(window.sshcDemo)), false);
  console.log("PASS: latestは確認し、デモtar.gzとVM実行は起動のクリックを待つ");
  await page.getByRole("button", { name: "起動する", exact: true }).click();
  await page.waitForURL(`**/github-releases/${manifest.version}/index.html`);
  await page.waitForFunction(() => window.sshcDemo?.machines.length === 3);
  assert.equal(await page.locator("#demo-version").innerText(), manifest.version);
  assert.equal(archiveRequests, 1);
  await page.evaluate(() => {
    window.releaseCheckOutput = "";
    const decoder = new TextDecoder();
    window.sshcDemo.machines[0].add_listener("serial0-output-byte", byte => {
      window.releaseCheckOutput += decoder.decode(Uint8Array.of(byte), { stream: true });
    });
  });
  await page.locator("#status").filter({ hasText: "Web UIとCLIを操作できます" }).waitFor();
  await page.evaluate(() => {
    for (const byte of new TextEncoder().encode("sshc version\n")) window.sshcDemo.machines[0].bus.send("serial0-input", byte);
  });
  await page.waitForFunction(version => window.releaseCheckOutput.includes(`sshc ${version} linux/386`), manifest.version);
  await page.screenshot({ path: resolve(artifactDirectory, "release-ready.png") });
  console.log("PASS: 展開したReleaseのVM・Web UI・CLIが起動し、実バイナリと画面が同じ版を報告する");
  await page.goto(demoURL);
  await page.getByRole("button", { name: "起動する", exact: true }).click();
  await page.waitForURL(`**/github-releases/${manifest.version}/index.html`);
  await page.locator("#clear-ui-cache").waitFor({ state: "visible" });
  assert.equal(archiveRequests, 1);
  console.log("PASS: 2回目は展開済みのReleaseを再利用し、tar.gzを取得しない");
  await page.locator("#clear-ui-cache").click();
  await page.waitForURL(demoURL);
  await page.getByRole("button", { name: "起動する", exact: true }).waitFor();
  assert.deepEqual(await page.evaluate(() => caches.keys()), []);
  assert.equal(await page.evaluate(() => Boolean(window.sshcDemo)), false);
  console.log("PASS: キャッシュ削除でUIとReleaseの束を両方消し、起動前の入口に戻る");
  await context.close();

  const fallbackContext = await browser.newContext({ locale: "ja-JP" });
  await fallbackContext.route(`${origin}/config.json`, async route => {
    const response = await route.fetch();
    await route.fulfill({ json: { ...await response.json(), releaseProxyURL: `${origin}/` } });
  });
  await fallbackContext.route(`${origin}/latest.json`, route => route.fulfill({ status: 502, json: { error: "Unavailable" } }));
  const fallbackPage = await fallbackContext.newPage();
  await fallbackPage.goto(demoURL);
  await fallbackPage.locator("#release-status").filter({ hasText: "配信済み" }).waitFor();
  await fallbackPage.locator("#start").click();
  await fallbackPage.waitForFunction(() => window.sshcDemo?.machines.length === 3);
  assert.equal(await fallbackPage.locator("#demo-version").innerText(), manifest.version);
  console.log("PASS: latestの取得失敗時は配信済みの版を起動する");
  await fallbackContext.close();
} finally {
  await browser.close();
}

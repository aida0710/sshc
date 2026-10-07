import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { chromium } from "playwright";

const demoURL = process.env.SSHC_DEMO_TEST_URL ?? "http://127.0.0.1:4178/index.html";
const artifactDirectory = resolve(process.env.SSHC_DEMO_ARTIFACTS ?? "../artifacts/demo/browser-check");
await mkdir(artifactDirectory, { recursive: true });
const browser = await chromium.launch({
  ...(process.env.SSHC_DEMO_CHROMIUM ? { executablePath: process.env.SSHC_DEMO_CHROMIUM } : {}),
  headless: true,
});
let page;
try {
  page = await browser.newPage({ viewport: { width: 1440, height: 980 }, locale: "ja-JP" });
  page.setDefaultTimeout(60_000);
  const imageRequests = [];
  const browserErrors = [];
  const archiveRequests = [];
  const uiResponses = [];
  page.on("request", (request) => {
    if (/\.(?:bin|cpio\.gz|wasm)(?:\?|$)/.test(request.url())) imageRequests.push(request.url());
    if (request.url().endsWith("/ui.tar.gz")) archiveRequests.push(request.url());
  });
  page.on("response", (response) => {
    if (new URL(response.url()).pathname.includes("/ui/")) {
      uiResponses.push({ status: response.status(), fromCacheWorker: response.fromServiceWorker() });
    }
  });
  // Avoid printing URLs or session tokens when a runtime error occurs.
  page.on("pageerror", (error) => browserErrors.push(error.name));
  await page.goto(demoURL);
  await page.getByRole("button", { name: "起動する", exact: true }).waitFor();
  assert.match(await page.locator("#updated-at").innerText(), /\d{4}\/\d{2}\/\d{2}.*JST/);
  assert.ok(Number.isFinite(await page.locator("#updated-at").evaluate((time) => Date.parse(time.dateTime))));
  assert.equal(await page.evaluate(() => Boolean(window.sshcDemo)), false);
  assert.equal(imageRequests.length, 0, "VM assets must wait for consent");
  assert.equal(archiveRequests.length, 0, "UI archive must wait for consent");
  await page.locator("#clear-ui-cache").waitFor({ state: "hidden" });
  await page.screenshot({ path: resolve(artifactDirectory, "confirmation.png") });
  console.log("PASS: 起動を選ぶまでVMイメージを取得しない");

  const startedAt = Date.now();
  await page.getByRole("button", { name: "起動する", exact: true }).click();
  await page.locator("#startup").waitFor({ state: "visible" });
  assert.equal(await page.locator(".startup-machines li").count(), 3);
  await page.waitForFunction(() => window.sshcDemo?.machines.length === 3);
  await page.evaluate(() => {
    window.demoVerificationCLI = "";
    const decoder = new TextDecoder();
    window.sshcDemo.machines[0].add_listener("serial0-output-byte", (byte) => {
      window.demoVerificationCLI += decoder.decode(Uint8Array.of(byte), { stream: true });
    });
  });
  const ui = page.frameLocator("#sshc-ui");
  await page.locator('.startup-machines li[data-state="booting"]').first().waitFor();
  await page.screenshot({ path: resolve(artifactDirectory, "startup-progress.png") });
  await ui.getByRole("button", { name: /demo-a/ }).first().waitFor({ timeout: 150_000 });
  await page.locator("#startup").waitFor({ state: "hidden" });
  assert.equal(await page.locator("#startup-download").evaluate((bar) => bar.value), 100);
  assert.equal(await page.locator('.startup-machines li[data-state="ready"]').count(), 3);
  console.log(`PASS: VM3台とWeb UI起動（${Math.round((Date.now() - startedAt) / 1000)}秒）`);

  await page.getByRole("button", { name: "CLI", exact: true }).click();
  const cliKeyboard = page.locator("#cli .xterm-helper-textarea");
  await cliKeyboard.focus();
  await page.keyboard.insertText("sshc ssh demo-a --non-interactive -- hostname; sshc ssh demo-b --non-interactive -- hostname");
  await page.keyboard.press("Enter");
  await page.waitForFunction(() => /\ndemo-a\r?\n/.test(window.demoVerificationCLI) && /\ndemo-b\r?\n/.test(window.demoVerificationCLI), null, { timeout: 90_000 });
  await page.screenshot({ path: resolve(artifactDirectory, "cli.png") });
  console.log("PASS: CLIからdemo-aとdemo-bにSSH接続");

  await page.getByRole("button", { name: "Web UI", exact: true }).click();
  const uiFrame = await (await page.locator("#sshc-ui").elementHandle()).contentFrame();
  await uiFrame.evaluate(() => {
    window.demoVerificationWebOutput = "";
    const Transport = window.WebSocket;
    window.WebSocket = class extends Transport {
      constructor(...parameters) {
        super(...parameters);
        const decoder = new TextDecoder();
        this.addEventListener("message", (event) => {
          if (event.data instanceof ArrayBuffer) {
            window.demoVerificationWebOutput += decoder.decode(event.data, { stream: true });
          }
        });
      }
    };
  });
  await ui.getByRole("button", { name: /demo-a/ }).first().dblclick();
  const webKeyboard = ui.locator(".xterm-helper-textarea").first();
  await webKeyboard.waitFor();
  await ui.getByRole("status").filter({ hasText: /^接続済み$/ }).waitFor();
  await uiFrame.waitForFunction(() => window.demoVerificationWebOutput.includes("#"));
  await webKeyboard.focus();
  await page.keyboard.insertText("printf 'WEB_%s:%s\\n' A \"$(hostname)\"");
  await page.keyboard.press("Enter");
  await uiFrame.waitForFunction(() => window.demoVerificationWebOutput.includes("WEB_A:demo-a"));
  await page.screenshot({ path: resolve(artifactDirectory, "web-terminal.png") });
  console.log("PASS: WebターミナルでSSH接続とコマンド実行");

  await ui.getByRole("link", { name: "SFTP", exact: true }).click();
  const pane = ui.getByRole("tabpanel");
  await pane.locator("button[data-value]:visible").click();
  await ui.getByRole("dialog").getByText("demo-a", { exact: true }).click();
  await pane.getByRole("button", { name: "接続", exact: true }).click();
  await ui.getByRole("button", { name: "welcome.txt", exact: true }).dblclick();
  await ui.getByRole("dialog", { name: "welcome.txtの詳細" }).getByRole("button", { name: "ファイルを編集" }).click();
  const editor = ui.getByRole("dialog", { name: "/root/welcome.txt", exact: true });
  const editorContent = editor.getByRole("textbox", { name: "Editor content" });
  await editorContent.focus();
  await page.keyboard.press("Control+a");
  const contents = "ブラウザ内のSFTPで保存しました。\nHello from sshc demo!\n";
  await page.keyboard.insertText(contents);
  await editor.getByText("未保存", { exact: true }).waitFor();
  await editor.getByRole("button", { name: "保存", exact: true }).click();
  // A click starts the write; only the cleared dirty state confirms that it completed.
  await editor.getByText("未保存", { exact: true }).waitFor({ state: "hidden" });
  const iframe = await page.locator("#sshc-ui").contentFrame().locator("body").elementHandle();
  const saved = await iframe.evaluate(async () => {
    const response = await fetch("/api/v1/sftp/demo-a/text?path=%2Froot%2Fwelcome.txt", {
      headers: { "X-SSHC-CSRF": sessionStorage.getItem("sshc.session.csrf") },
    });
    if (!response.ok) throw new Error(`SFTP read returned ${response.status}`);
    return response.json();
  });
  assert.equal(saved.contents, contents);
  await page.screenshot({ path: resolve(artifactDirectory, "sftp-editor.png") });
  console.log("PASS: SFTPで一覧・編集・日本語の保存と再読み込み");
  assert.equal(archiveRequests.length, 1);
  await page.locator("#clear-ui-cache").waitFor({ state: "visible" });
  assert.ok(uiResponses.length > 10);
  assert.ok(uiResponses.every((response) => response.status === 200 && response.fromCacheWorker));
  console.log("PASS: UIはtar.gzを1回取得し、画面・エディタ・フォントをブラウザ内から読む");
  assert.deepEqual(browserErrors, []);
  await page.getByRole("button", { name: "最初からやり直す" }).click();
  await page.getByRole("button", { name: "起動する", exact: true }).waitFor();
  assert.equal(await page.evaluate(() => Boolean(window.sshcDemo)), false);
  console.log("PASS: リセットするとVMを破棄し、起動前の確認に戻る");
  await page.route("**/ui.tar.gz", (route) => route.abort());
  await page.evaluate(async () => {
    const { prepareUIArchive } = await import("./ui-archive.js");
    const configuration = await (await fetch("./config.json")).json();
    await prepareUIArchive({ archive: configuration.uiArchive, baseURL: new URL("./ui/", location.href),
      onProgress: () => {}, });
  });
  assert.equal(archiveRequests.length, 1);
  assert.equal(await page.evaluate(() => Boolean(window.sshcDemo)), false);
  console.log("PASS: 次回は展開済みUIを再利用し、アーカイブを取り直さない");
  const cacheName = await page.evaluate(() => `sshc-demo-ui:${new URL('./ui/', location.href).href}`);
  await page.getByRole("button", { name: "キャッシュを削除して再読み込み", exact: true }).click();
  await page.getByRole("button", { name: "起動する", exact: true }).waitFor();
  await page.locator("#clear-ui-cache").waitFor({ state: "hidden" });
  assert.equal(await page.evaluate((name) => caches.has(name), cacheName), false);
  assert.equal(await page.evaluate(() => Boolean(window.sshcDemo)), false);
  console.log("PASS: UIキャッシュを削除して公開版の入口へ戻り、VMは起動前の状態になる");
} catch (error) {
  await page?.screenshot({ path: resolve(artifactDirectory, "failure.png") });
  console.log("Failure UI:", await page?.frameLocator("#sshc-ui").locator("body").innerText().catch(() => "unavailable"));
  throw error;
} finally {
  await browser.close();
}

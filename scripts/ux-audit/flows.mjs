// sshc の UI/UX 点検（2026-11-03）の「作業の流れ」を実際に流し、手数・待ち時間を記録して動画に撮る。
// 使い方: node scripts/ux-audit/flows.mjs <出力先> <流れ: F1,F3,F4,...>
// 前提: bin/sshc と、docker のイメージ sshc-ux-audit-sshd:2026-10-30。
// 流れごとに一時の HOME とエンジンを作り直す（前の流れの状態を持ち越さない）。
import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { chromium } from "../../web/node_modules/playwright/index.mjs";

const repository = resolve(new URL("../..", import.meta.url).pathname);
const out = resolve(process.argv[2] ?? "artifacts/ux-audit/flows");
const wanted = new Set((process.argv[3] ?? "F1").split(","));
const scratch = process.env.UX_AUDIT_SCRATCH ?? "/tmp";
const enginePort = process.env.UX_AUDIT_ENGINE_PORT ?? "29530";
const sshdPort = process.env.UX_AUDIT_SSHD_PORT ?? "29396";
const sshdContainer = process.env.UX_AUDIT_SSHD_CONTAINER ?? "sshc-ux-flows-sshd";
const sshdImage = "sshc-ux-audit-sshd:2026-10-30";
// 撮影のためだけの使い捨ての値（実在の認証情報ではない）。
const masterPassword = "an audit master password for screenshots";
const serverPassword = "fixture-password";
mkdirSync(out, { recursive: true });

const jaSource = readFileSync(join(repository, "web/src/i18n/messages/ja.ts"), "utf8");
function ja(key) {
  const match = jaSource.match(new RegExp(`"${key.replace(/\./g, "\\.")}":\\s*"([^"]*)"`));
  if (match === null) throw new Error(`ja.ts has no ${key}`);
  return match[1];
}
const run = (command, args, options = {}) => execFileSync(command, args, { encoding: "utf8", ...options });
const pause = (seconds) => run("sleep", [String(seconds)]);

// ---- 使い捨ての sshd（鍵とパスワードの両方で入れる） ----
function startSSHServer(publicKeys) {
  try { run("docker", ["rm", "-f", sshdContainer], { stdio: "ignore" }); } catch { /* 無ければよい */ }
  run("docker", ["run", "-d", "--name", sshdContainer, "--hostname", "web-01", "-p", `127.0.0.1:${sshdPort}:22`, sshdImage]);
  run("docker", ["exec", "-i", sshdContainer, "sh", "-c", [
    "cat > /home/deploy/.ssh/authorized_keys && chown deploy:deploy /home/deploy/.ssh/authorized_keys && chmod 600 /home/deploy/.ssh/authorized_keys",
    `echo 'deploy:${serverPassword}' | chpasswd`,
    // 撮影用のイメージはパスワード認証を sshd_config.d/10-fixture.conf で切っている。
    "sed -i 's/^PasswordAuthentication no/PasswordAuthentication yes/' /etc/ssh/sshd_config.d/10-fixture.conf /etc/ssh/sshd_config",
    "kill -HUP 1",
  ].join(" && ")], { input: publicKeys.join("") });
  for (let attempt = 0; attempt < 50; attempt++) {
    try {
      const scanned = run("ssh-keyscan", ["-p", sshdPort, "-t", "ed25519", "127.0.0.1"], { stdio: ["ignore", "pipe", "ignore"] }).trim();
      if (scanned !== "") return scanned;
    } catch { /* まだ起動していない */ }
    pause(0.2);
  }
  throw new Error("the fixture sshd did not answer");
}

// ---- 一時の HOME ----
const hostBlock = (alias, lines) => [`Host ${alias}`, ...lines.map((line) => `\t${line}`), ""].join("\n");

function buildHome({ empty }) {
  const home = mkdtempSync(join(scratch, "ux-flow-home-"));
  const ssh = join(home, ".ssh");
  mkdirSync(ssh, { recursive: true, mode: 0o700 });
  writeFileSync(join(home, ".bashrc"), "export PS1='$ '\nexport LANG=C.UTF-8\n");
  writeFileSync(join(home, ".bash_profile"), ". ~/.bashrc\n");
  if (empty) return { home, publicKeys: [] };
  run("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-C", "deploy@workstation", "-f", join(ssh, "id_ed25519_deploy")]);
  run("ssh-keygen", ["-q", "-t", "ed25519", "-N", "fixture passphrase", "-C", "github@workstation", "-f", join(ssh, "id_ed25519_passphrase")]);
  writeFileSync(join(ssh, "config"), [
    hostBlock("web-01", ["HostName 127.0.0.1", `Port ${sshdPort}`, "User deploy", "IdentityFile ~/.ssh/id_ed25519_deploy", "IdentitiesOnly yes"]),
    hostBlock("password-only", ["HostName 127.0.0.1", `Port ${sshdPort}`, "User deploy", "PubkeyAuthentication no"]),
    hostBlock("key-with-passphrase", ["HostName 127.0.0.1", `Port ${sshdPort}`, "User deploy", "IdentityFile ~/.ssh/id_ed25519_passphrase", "IdentitiesOnly yes"]),
  ].join("\n"), { mode: 0o600 });
  return { home, publicKeys: ["id_ed25519_deploy.pub", "id_ed25519_passphrase.pub"].map((name) => readFileSync(join(ssh, name), "utf8")) };
}

// ---- エンジン ----
function startEngine(home, port = enginePort) {
  const ssh = join(home, ".ssh");
  mkdirSync(join(ssh, "sshc"), { recursive: true, mode: 0o700 });
  writeFileSync(join(ssh, "sshc", "engine-settings.json"), JSON.stringify({ schemaVersion: 1, port: Number(port) }) + "\n", { mode: 0o600 });
  const binDirectory = join(scratch, "bin");
  mkdirSync(binDirectory, { recursive: true });
  run("install", ["-m", "0755", join(repository, "bin", "sshc"), join(binDirectory, "sshc")]);
  const environment = { PATH: `${binDirectory}:${process.env.PATH ?? ""}`, HOME: home, TERM: "xterm-256color", LANG: "C.UTF-8", SHELL: "/bin/bash" };
  // 同期の流れ（F8）は自己署名の証明書の S3 互換のサーバーを使う。エンジンにその CA を信頼させる。
  if (process.env.UX_AUDIT_CA_FILE !== undefined) environment.SSL_CERT_FILE = process.env.UX_AUDIT_CA_FILE;
  const engine = spawn(join(binDirectory, "sshc"), ["engine", "--port", String(port)], { env: environment, stdio: ["ignore", "pipe", "pipe"] });
  let log = "";
  engine.stdout.on("data", (chunk) => { log += chunk; });
  engine.stderr.on("data", (chunk) => { log += chunk; });
  for (let attempt = 0; attempt < 300; attempt++) {
    try { run("bash", ["-c", `exec 3<>/dev/tcp/127.0.0.1/${port}`], { stdio: "ignore" }); break; } catch { pause(0.1); }
  }
  const openURL = () => {
    for (let attempt = 0; attempt < 150; attempt++) {
      try {
        const printed = run("sshc", ["open", "--no-browser"], { env: environment, stdio: ["ignore", "pipe", "ignore"] }).trim();
        const url = printed.split(/\s+/).find((word) => word.startsWith("http://127.0.0.1"));
        if (url !== undefined) return url;
      } catch { /* まだ */ }
      pause(0.1);
    }
    throw new Error("sshc open printed no URL");
  };
  return { engine, openURL, log: () => log };
}

// ---- 記録 ----
// Recorder は、利用者の操作（手数）と待ち時間を流れごとに記録し、要所で画面を撮る。
function recorder(flowId, page) {
  const started = Date.now();
  const steps = [];
  let actions = 0;
  const seconds = () => Number(((Date.now() - started) / 1000).toFixed(1));
  return {
    async act(label, action) {
      actions += 1;
      const at = seconds();
      try { await action(); steps.push({ at, actions, label }); console.log(flowId, at, "act", label); }
      catch (error) { steps.push({ at, actions, label, failed: String(error).split("\n")[0] }); console.log(flowId, at, "FAILED", label, String(error).split("\n")[0]); }
    },
    async waitFor(label, condition, timeout = 60000) {
      const at = seconds();
      try { await condition(timeout); steps.push({ at, waited: Number((seconds() - at).toFixed(1)), label }); console.log(flowId, seconds(), "waited", label); }
      catch (error) { steps.push({ at, label, timedOut: String(error).split("\n")[0] }); console.log(flowId, seconds(), "TIMEOUT", label); }
    },
    async shot(name) {
      await page.screenshot({ path: join(out, `${flowId}-${name}.png`) }).catch(() => {});
      steps.push({ at: seconds(), shot: `${flowId}-${name}.png` });
    },
    note(text) { steps.push({ at: seconds(), note: text }); console.log(flowId, seconds(), "note", text); },
    finish() { writeFileSync(join(out, `${flowId}.json`), JSON.stringify({ flow: flowId, actions, seconds: seconds(), steps }, null, 2)); },
  };
}

async function newContext(browser, { phone = false } = {}) {
  const viewport = phone ? { width: 390, height: 844 } : { width: 1600, height: 900 };
  const context = await browser.newContext({
    viewport, locale: "ja-JP", timezoneId: "Asia/Tokyo",
    ...(phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}),
    recordVideo: { dir: join(out, "video"), size: viewport },
  });
  await context.route(/\/api\/v1\/update(?:\/check)?$/, (route) => route.fulfill({ json: { current: "dev", available: false, canUpdate: false } }));
  return context;
}

// signIn は Vault を作る（まだ無ければ）か解除して、ナビゲーションが出るまで待つ。手数には数えない（F1 だけは数える）。
async function signIn(page, url) {
  await page.goto(url);
  const fields = page.locator('input[type="password"]');
  const navigation = page.locator(`nav[aria-label="${ja("shell.primaryNavigation")}"]:visible, nav[aria-label="${ja("shell.mobileNavigation")}"]:visible`).first();
  await fields.first().or(navigation).waitFor({ timeout: 20000 });
  await page.waitForTimeout(1500);
  if (await fields.first().isVisible()) {
    for (let index = 0; index < await fields.count(); index++) await fields.nth(index).fill(masterPassword);
    await page.locator('button[type="submit"]').click();
    await fields.first().waitFor({ state: "hidden", timeout: 20000 });
  }
  await navigation.waitFor({ timeout: 20000 });
  await page.waitForTimeout(1000);
}

const go = (page, path) => page.evaluate((target) => { window.history.pushState(null, "", target); window.dispatchEvent(new PopStateEvent("popstate")); }, path);
const terminalText = (page) => page.locator(".xterm-rows").first().innerText().catch(() => "");

// ---- 流れ ----

// F1: 初めて起動して最初の接続をする（~/.ssh が空、パスワードで入るサーバー）。
async function flowFirstConnection(browser, environment) {
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F1", page);
  await page.goto(environment.openURL());
  await page.locator('input[type="password"]').first().waitFor({ timeout: 20000 });
  await record.shot("01-entry");
  const fields = page.locator('input[type="password"]');
  await record.act("マスターパスワードを入れる", () => fields.nth(0).fill(masterPassword));
  await record.act("確認の欄に入れる", () => fields.nth(1).fill(masterPassword));
  await record.act("［Vaultを作成］", () => page.locator('button[type="submit"]').click());
  await record.waitFor("Home が出る", (timeout) => page.locator(`nav[aria-label="${ja("shell.primaryNavigation")}"]:visible`).first().waitFor({ timeout }));
  await page.waitForTimeout(1500);
  await record.shot("02-home-empty");
  // 空の Home には接続先を足すボタンが無い（2026-11-03 の所見）。見出しの［Connectionsを管理］から進む。
  await record.act("［Connectionsを管理］", () => page.getByRole("button", { name: /Connectionsを管理/ }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1200);
  await record.shot("02b-connections-empty");
  await record.act("［接続先を追加］", () => page.getByRole("button", { name: /接続先を追加/ }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1200);
  await record.shot("03-create-form");
  await record.act("接続名を入れる", () => page.getByLabel(ja("conn.createName"), { exact: true }).fill("my-server"));
  await record.act("ホスト名を入れる", () => page.getByLabel(ja("conn.createHostName"), { exact: true }).fill("127.0.0.1"));
  await record.act("ユーザーを入れる", () => page.getByLabel(ja("conn.createUser"), { exact: true }).fill("deploy"));
  await record.act("ポートを入れる", () => page.getByLabel(ja("conn.createPort"), { exact: true }).fill(sshdPort));
  await record.shot("04-create-filled");
  await record.act("認証方法をパスワードにする", () => page.getByLabel(ja("conn.createAuthenticationMethod"), { exact: true }).selectOption("dedicated_password"));
  await page.waitForTimeout(800);
  await record.shot("05-create-password");
  // パスワードの欄（Vault の解除が要るなら先に出る）。出ている password の欄に順に入れる。
  const passwordFields = page.getByRole("main").locator('input[type="password"]:visible');
  const count = await passwordFields.count();
  record.note(`パスワードの欄の数: ${count}`);
  for (let index = 0; index < count; index++) {
    await record.act(`パスワードの欄 ${index + 1} に入れる`, () => passwordFields.nth(index).fill(serverPassword));
  }
  await record.shot("06-create-password-filled");
  await record.act("［接続先を作成］", () => page.getByRole("button", { name: ja("conn.create"), exact: true }).click({ timeout: 8000 }));
  await page.waitForTimeout(2000);
  await record.shot("07-created");
  await record.act("［接続］", () => page.getByRole("button", { name: /^接続$/ }).first().click({ timeout: 8000 }));
  await record.waitFor("ホスト鍵の確認が出る", (timeout) => page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first().waitFor({ timeout }), 30000);
  await record.shot("08-host-key");
  await record.act("［接続する］（ホスト鍵を受け入れる）", () => page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first().click({ timeout: 8000 }));
  await record.waitFor("プロンプトが出る", async (timeout) => {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) { if ((await terminalText(page)).includes("deploy@web-01")) return; await page.waitForTimeout(500); }
    throw new Error("no prompt");
  }, 30000);
  await page.waitForTimeout(800);
  await record.shot("09-connected");
  record.finish();
  await context.close();
}

// F3: パスワードを保存していない接続先と、パスフレーズ付きの鍵で接続する。
async function flowAuthentication(browser, environment) {
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F3", page);
  await signIn(page, environment.openURL());
  for (const [alias, secret] of [["password-only", serverPassword], ["key-with-passphrase", "fixture passphrase"]]) {
    await go(page, "/connections");
    await page.waitForTimeout(1000);
    await record.act(`${alias} を選ぶ`, () => page.getByRole("main").getByText(alias, { exact: true }).first().click({ timeout: 8000 }));
    await page.waitForTimeout(800);
    await record.act(`${alias} に［接続］`, () => page.getByRole("button", { name: /^接続$/ }).first().click({ timeout: 8000 }));
    await page.waitForTimeout(4000);
    await record.shot(`${alias}-01-after-connect`);
    const accept = page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first();
    if (await accept.isVisible().catch(() => false)) {
      await record.act("［接続する］（ホスト鍵）", () => accept.click());
      await page.waitForTimeout(3000);
      await record.shot(`${alias}-02-after-host-key`);
    }
    record.note(`${alias} のターミナル: ${(await terminalText(page)).replace(/\s+/g, " ").slice(-300)}`);
    record.note(`${alias} の見える欄: ${(await page.locator('input:visible').evaluateAll((inputs) => inputs.map((input) => `${input.type}:${input.getAttribute("aria-label") ?? input.placeholder}`))).join(", ")}`);
    // 画面に秘密の入力欄が出ていればそこへ、無ければターミナルへ打つ。
    const secretField = page.locator('input[type="password"]:visible').first();
    if (await secretField.isVisible().catch(() => false)) {
      await record.act(`${alias} の秘密を入力欄へ`, () => secretField.fill(secret));
      await record.shot(`${alias}-03-secret-field`);
      await record.act("Enter", () => page.keyboard.press("Enter"));
    } else {
      await record.act(`${alias} の秘密をターミナルへ打つ`, async () => { await page.locator(".xterm-screen").first().click(); await page.keyboard.type(secret); await page.keyboard.press("Enter"); });
    }
    await page.waitForTimeout(4000);
    await record.shot(`${alias}-04-result`);
    record.note(`${alias} の結果: ${(await terminalText(page)).replace(/\s+/g, " ").slice(-200)}`);
  }
  record.finish();
  await context.close();
}

// F4: 接続中にサーバーが止まる → 戻る（再接続）→ もう一度止まって戻らない（失敗）。
async function flowReconnect(browser, environment) {
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F4", page);
  await signIn(page, environment.openURL());
  await go(page, "/connections");
  await page.waitForTimeout(1000);
  await page.getByRole("main").getByText("web-01", { exact: true }).first().click({ timeout: 8000 });
  await page.getByRole("button", { name: /^接続$/ }).first().click({ timeout: 8000 });
  await page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first().click({ timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(3000);
  await record.shot("01-connected");
  record.note("サーバーを止める（docker stop）");
  run("docker", ["stop", "-t", "0", sshdContainer]);
  for (let tick = 1; tick <= 8; tick++) { await page.waitForTimeout(3000); await record.shot(`02-down-${String(tick * 3).padStart(2, "0")}s`); }
  record.note("サーバーを戻す（docker start）");
  run("docker", ["start", sshdContainer]);
  for (let tick = 1; tick <= 8; tick++) { await page.waitForTimeout(3000); await record.shot(`03-up-${String(tick * 3).padStart(2, "0")}s`); }
  record.note("サーバーを止めたままにする");
  run("docker", ["stop", "-t", "0", sshdContainer]);
  for (let tick = 1; tick <= 30; tick++) { await page.waitForTimeout(6000); if (tick % 3 === 0) await record.shot(`04-gone-${String(tick * 6).padStart(3, "0")}s`); }
  record.finish();
  await context.close();
  run("docker", ["start", sshdContainer]);
}

// openConnected は Connections から接続先を開いてつなぎ、ホスト鍵を受け入れる（流れの前置き。手数に数えない）。
async function openConnected(page, alias) {
  await go(page, "/connections");
  await page.waitForTimeout(1000);
  await page.getByRole("main").getByText(alias, { exact: true }).first().click({ timeout: 8000 });
  await page.getByRole("button", { name: /^接続$/ }).first().click({ timeout: 8000 });
  await page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first().click({ timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(3000);
}

// F2: 踏み台を経由する接続先を足して接続する（踏み台は web-01、行き先はコンテナの中の 127.0.0.1:22）。
async function flowJumpHost(browser, environment) {
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F2", page);
  await signIn(page, environment.openURL());
  await openConnected(page, "web-01");
  await go(page, "/connections");
  await page.waitForTimeout(1000);
  await record.act("［接続先を追加］", () => page.getByRole("button", { name: /接続先を追加/ }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1000);
  await record.act("接続名を入れる", () => page.getByLabel(ja("conn.createName"), { exact: true }).fill("inner"));
  await record.act("ホスト名を入れる", () => page.getByLabel(ja("conn.createHostName"), { exact: true }).fill("127.0.0.1"));
  await record.act("ユーザーを入れる", () => page.getByLabel(ja("conn.createUser"), { exact: true }).fill("deploy"));
  await record.shot("01-create-form");
  record.note(`作成の画面に踏み台の欄があるか: ${await page.getByText(/踏み台|経由/).count()}`);
  await record.act("［接続先を作成］", () => page.getByRole("button", { name: ja("conn.create"), exact: true }).click({ timeout: 8000 }));
  await page.waitForTimeout(2000);
  await record.act("［踏み台・転送・その他］のタブ", () => page.getByRole("tab", { name: ja("conn.areaAdvanced"), exact: true }).click({ timeout: 8000 }));
  await page.waitForTimeout(800);
  await record.shot("02-advanced-tab");
  const jumpField = page.getByRole("main").locator("select, input[role=combobox], button[aria-haspopup]").first();
  record.note(`踏み台の部品: ${await jumpField.evaluate((element) => `${element.tagName} ${element.getAttribute("aria-label") ?? ""}`).catch(() => "見つからない")}`);
  await record.act("経由する接続先で web-01 を選ぶ", () => page.getByRole("main").locator("select").first().selectOption({ label: "web-01" }));
  await page.waitForTimeout(800);
  await record.shot("04-jump-chosen");
  await record.act("［保存］", () => page.getByRole("button", { name: /^(保存|変更を保存)$/ }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1500);
  await record.act("［接続］", () => page.getByRole("button", { name: /^接続$/ }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(4000);
  await record.shot("05-after-connect");
  const accept = page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first();
  if (await accept.isVisible().catch(() => false)) { await record.act("［接続する］（ホスト鍵）", () => accept.click()); await page.waitForTimeout(3000); }
  await record.shot("06-result");
  record.note(`結果: ${(await terminalText(page)).replace(/\s+/g, " ").slice(-200)}`);
  record.finish();
  await context.close();
}

// F5: 分割表示（右に分割・下に分割）とワークスペース。
async function flowSplit(browser, environment) {
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F5", page);
  await signIn(page, environment.openURL());
  await openConnected(page, "web-01");
  await record.act("ターミナルの［…］", () => page.getByRole("button", { name: ja("terminal.moreActions") }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(600);
  await record.shot("01-more");
  await record.act("［右に分割して表示］", () => page.getByText(ja("splitView.right"), { exact: true }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1500);
  await record.shot("02-after-split-right");
  record.note(`本文: ${(await page.getByRole("main").innerText().catch(() => "")).replace(/\s+/g, " ").slice(0, 400)}`);
  record.finish();
  await context.close();
}

// F6: SFTP でファイルを上げる・落とす（速度の上限を 2 MiB/s にして、進み具合の出方を見る）。
async function flowSFTP(browser, environment, home) {
  writeFileSync(join(home, "upload-30MiB.bin"), Buffer.alloc(30 * 1024 * 1024, 7));
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F6", page);
  await signIn(page, environment.openURL());
  // SFTP の画面ではホスト鍵を受け入れられない（2026-11-03 の所見）。先にターミナルで受け入れておく。
  await openConnected(page, "web-01");
  await go(page, "/files");
  await page.waitForTimeout(1200);
  await record.act("［接続先を選択］", () => page.getByRole("button", { name: ja("sftp.chooseHost"), exact: true }).last().click({ timeout: 8000 }));
  await page.waitForTimeout(800);
  await record.act("web-01 を選ぶ", () => page.getByRole("dialog").getByText("web-01", { exact: true }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(2500);
  const accept = page.getByRole("button", { name: ja("terminal.hostKeyAccept"), exact: true }).first();
  if (await accept.isVisible().catch(() => false)) { await record.act("［接続する］（ホスト鍵）", () => accept.click()); await page.waitForTimeout(2500); }
  await record.shot("01-connected");
  record.note(`アップロードの入口: ${(await page.getByText(ja("sftp.upload"), { exact: true }).count())} 個、file の input: ${await page.locator('input[type=file]').count()} 個`);
  await record.act("［アップロード］（file の input に渡す）", () => page.locator('input[type=file]:not([webkitdirectory])').first().setInputFiles(join(home, "upload-30MiB.bin")));
  for (let tick = 1; tick <= 4; tick++) { await page.waitForTimeout(1500); await record.shot(`02-upload-${tick}`); }
  await record.act("backups を開く", () => page.getByRole("main").getByText("backups", { exact: true }).first().dblclick({ timeout: 8000 }));
  await page.waitForTimeout(1500);
  await record.act("ファイルを選ぶ", () => page.getByRole("main").getByText("db-2026-10-29.sql.gz", { exact: true }).first().click({ timeout: 8000 }));
  await record.act("［ダウンロード］", () => page.getByRole("button", { name: ja("sftp.download"), exact: true }).first().click({ timeout: 8000 }));
  for (let tick = 1; tick <= 4; tick++) { await page.waitForTimeout(1500); await record.shot(`03-download-${tick}`); }
  record.finish();
  await context.close();
}

// F7: スニペットを作り、ターミナルのクイックコマンドから変数を入れて実行する。
async function flowSnippet(browser, environment) {
  const context = await newContext(browser);
  const page = await context.newPage();
  const record = recorder("F7", page);
  await signIn(page, environment.openURL());
  await go(page, "/snippets");
  await page.waitForTimeout(1000);
  await record.act("［スニペットを追加］", () => page.getByRole("button", { name: /スニペットを追加/ }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1000);
  await record.shot("01-new-snippet");
  record.note(`欄: ${(await page.locator('input:visible, textarea:visible').evaluateAll((inputs) => inputs.map((input) => input.getAttribute("aria-label") ?? input.placeholder ?? input.name))).join(" / ")}`);
  await record.act("名前を入れる", () => page.getByRole("main").locator("input:visible").first().fill("ログの末尾"));
  await record.act("コマンドを入れる", () => page.getByRole("main").locator("textarea:visible").first().fill("tail -n {{lines}} /home/deploy/app/shared/log/app.log"));
  await page.waitForTimeout(800);
  await record.shot("02-filled");
  await record.act("［保存］", () => page.getByRole("button", { name: ja("snippets.save"), exact: true }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1200);
  await record.shot("03-saved");
  await openConnected(page, "web-01");
  await record.act("ターミナルの［…］", () => page.getByRole("button", { name: ja("terminal.moreActions") }).first().click({ timeout: 8000 }));
  await record.act("［クイックコマンド］", () => page.getByRole("menuitem", { name: ja("terminal.quickCommands") }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1000);
  await record.shot("04-quick-commands");
  await record.act("スニペットを選ぶ", () => page.getByText("ログの末尾", { exact: true }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(800);
  await record.shot("05-chosen");
  record.note(`クイックコマンドの欄: ${(await page.locator('input:visible').evaluateAll((inputs) => inputs.map((input) => input.getAttribute("aria-label") ?? input.placeholder))).join(" / ")}`);
  await record.act("［実行］", () => page.getByRole("button", { name: ja("terminal.quickCommandRun"), exact: true }).first().click({ timeout: 8000 }));
  await page.waitForTimeout(1500);
  await record.shot("06-after-run");
  record.note(`ターミナル: ${(await terminalText(page)).replace(/\s+/g, " ").slice(-300)}`);
  record.finish();
  await context.close();
}

// buttonsOf は画面に出ているボタンの名前を並べる（手順を探るため）。
const buttonsOf = (page) => page.getByRole("main").getByRole("button").evaluateAll((buttons) => buttons.filter((button) => button.offsetParent !== null).map((button) => `${button.textContent.trim().slice(0, 24)}${button.disabled ? "(押せない)" : ""}`));

// F8: 1台目で同期を設定して送り、2台目で同じバケットと鍵を設定して受け取る（S3 互換のサーバーは UX_AUDIT_S3_*）。
async function flowSync(browser, environment, home) {
  const s3 = { endpoint: process.env.UX_AUDIT_S3_ENDPOINT, bucket: process.env.UX_AUDIT_S3_BUCKET, key: process.env.UX_AUDIT_S3_KEY, secret: process.env.UX_AUDIT_S3_SECRET };
  const second = buildHome({ empty: true });
  const secondEngine = startEngine(second.home, String(Number(enginePort) + 1));
  // 1台目が作った共通の暗号化キー（画面に一度だけ出る）を2台目へ渡す。
  let sharedKey = "";
  try {
    for (const [machine, engine] of [["A", environment], ["B", secondEngine]]) {
      const context = await newContext(browser);
      const page = await context.newPage();
      const record = recorder(`F8${machine}`, page);
      await signIn(page, engine.openURL());
      await go(page, "/sync");
      await page.waitForTimeout(1500);
      await record.shot("01-sync");
      const field = (label) => page.getByRole("main").getByLabel(label, { exact: true });
      await record.act("エンドポイント", () => field("エンドポイント").fill(s3.endpoint));
      await record.act("バケット名", () => field("バケット名").fill(s3.bucket));
      await record.act("リージョン", () => field("リージョン").fill("us-east-1"));
      await record.act("アクセスキーID", () => field("アクセスキーID").fill(s3.key));
      await record.act("シークレットアクセスキー", () => field("シークレットアクセスキー").fill(s3.secret));
      await record.shot("02-filled");
      record.note(`ボタン: ${(await buttonsOf(page)).join(" / ")}`);
      // ここから先は画面に出たボタンを順に押して進む（同期の画面の手順を探るため、名前で決め打ちしない）。
      for (let round = 1; round <= 8; round++) {
        const buttons = await buttonsOf(page);
        const next = buttons.find((name) => !name.endsWith("(押せない)") && /確認|保存|次へ|設定|作成|送信|送る|受信|受け取|適用|同期/.test(name) && !/詳しく|表示|閉じる/.test(name));
        record.note(`${round}回目のボタン: ${buttons.join(" / ")}`);
        if (next === undefined) break;
        await record.act(`［${next}］`, () => page.getByRole("main").getByRole("button", { name: next, exact: true }).first().click({ timeout: 8000 }));
        await page.waitForTimeout(3000);
        await record.shot(`03-round-${round}`);
        const shown = (await page.getByRole("main").innerText()).match(/[0-9A-Z]{4}(?:-[0-9A-Z]{4}){5}/);
        if (machine === "A" && shown !== null && sharedKey === "") { sharedKey = shown[0]; record.note("共通の暗号化キーが画面に出た"); break; }
        const secretFields = page.getByRole("main").locator('input[type="password"]:visible');
        const empty = [];
        for (let index = 0; index < await secretFields.count(); index++) if ((await secretFields.nth(index).inputValue()) === "") empty.push(index);
        if (empty.length > 0) {
          record.note(`空の秘密の欄: ${empty.length} 個（共通の暗号化キーと見て、同じ値を入れる）`);
          for (const index of empty) await record.act("共通の暗号化キーを入れる", () => secretFields.nth(index).fill(sharedKey));
        }
      }
      await record.shot("04-final");
      record.note(`本文: ${(await page.getByRole("main").innerText()).replace(/\s+/g, " ").slice(0, 600)}`);
      record.finish();
      await context.close();
    }
  } finally {
    secondEngine.engine.kill("SIGTERM");
    writeFileSync(join(out, "F8B-engine.log"), secondEngine.log());
    if (process.env.UX_AUDIT_KEEP !== "1") rmSync(second.home, { recursive: true, force: true });
  }
}

const flows = {
  F1: { run: flowFirstConnection, empty: true },
  F2: { run: flowJumpHost, empty: false },
  F3: { run: flowAuthentication, empty: false },
  F4: { run: flowReconnect, empty: false },
  F5: { run: flowSplit, empty: false },
  F6: { run: flowSFTP, empty: false },
  F7: { run: flowSnippet, empty: false },
  F8: { run: flowSync, empty: false },
};

const browser = await chromium.launch({ args: ["--disable-webgl"] });
try {
  for (const [id, flow] of Object.entries(flows).filter(([id]) => wanted.has(id))) {
    const { home, publicKeys } = buildHome({ empty: flow.empty });
    startSSHServer(publicKeys);
    const engine = startEngine(home);
    try { await flow.run(browser, engine, home); } catch (error) { console.log(id, "flow failed", String(error).split("\n")[0]); }
    engine.engine.kill("SIGTERM");
    writeFileSync(join(out, `${id}-engine.log`), engine.log());
    pause(1);
    if (process.env.UX_AUDIT_KEEP !== "1") rmSync(home, { recursive: true, force: true });
  }
} finally {
  await browser.close();
  try { run("docker", ["rm", "-f", sshdContainer], { stdio: "ignore" }); } catch { /* 消えている */ }
}

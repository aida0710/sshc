// sshc の UI/UX 監査（2026-10-30）のための撮影。
// 架空のデータだけを入れた一時 HOME で sshc engine を起動し、フォームファクタごとに各画面を撮る。
// 画像と一緒に、画面ごとの数値（横のはみ出し、小さい押す場所、切れた文字、主の領域の幅）を manifest.json に残す。
//
// 使い方: node scripts/ux-audit/capture-screens.mjs <出力先> [撮る段: all|entry|routes|states|lock|options]
// options の段は UX_OPTIONS（案の定義のモジュール）を読んで撮る。all には含めず、options と書いたときだけ流す。
// 前提: bin/sshc を作ってあること。docker のイメージ sshc-ux-audit-sshd:2026-10-30 を作ってあること。
import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { chromium, devices } from "../../web/node_modules/playwright/index.mjs";

const repository = resolve(new URL("../..", import.meta.url).pathname);
const binary = join(repository, "bin", "sshc");
const out = resolve(process.argv[2] ?? "artifacts/ux-audit/capture");
const stages = new Set((process.argv[3] ?? "all").split(","));
const wants = (stage) => stages.has("all") || stages.has(stage);
const scratch = process.env.UX_AUDIT_SCRATCH ?? "/tmp";
// 撮影用のエンジンと sshd は 127.0.0.1 だけで待ち受ける。普段使いのエンジン（既定のポート）とぶつからない番号にする。
// 別の作業ツリーで同時に撮るときは、UX_AUDIT_ENGINE_PORT・UX_AUDIT_SSHD_PORT・UX_AUDIT_SSHD_CONTAINER と、
// バイナリを写す UX_AUDIT_SCRATCH を変える。
// 始めに同じ名前のコンテナを消すので、名前が同じだと相手の撮影を止めてしまう。
const enginePort = process.env.UX_AUDIT_ENGINE_PORT ?? "46420";
const sshdPort = process.env.UX_AUDIT_SSHD_PORT ?? "2299";
const sshdImage = "sshc-ux-audit-sshd:2026-10-30";
const sshdContainer = process.env.UX_AUDIT_SSHD_CONTAINER ?? "sshc-ux-audit-sshd";

mkdirSync(out, { recursive: true });

// 日本語の画面の名前は ja.ts から引く。英語の名前で探すと日本語の画面で見つからないため。
const jaSource = readFileSync(join(repository, "web/src/i18n/messages/ja.ts"), "utf8");
function ja(key) {
  const match = jaSource.match(new RegExp(`"${key.replace(/\./g, "\\.")}":\\s*"([^"]*)"`));
  if (match === null) throw new Error(`ja.ts has no ${key}`);
  return match[1];
}

// ---- 架空のデータ ----

function run(command, args, options = {}) {
  return execFileSync(command, args, { encoding: "utf8", ...options });
}

function startSSHServer(home) {
  try { run("docker", ["rm", "-f", sshdContainer], { stdio: "ignore" }); } catch { /* 無ければよい */ }
  run("docker", ["run", "-d", "--name", sshdContainer, "--hostname", "web-01", "-p", `127.0.0.1:${sshdPort}:22`, sshdImage]);
  const publicKey = readFileSync(join(home, ".ssh", "id_ed25519_deploy.pub"), "utf8");
  run("docker", ["exec", "-i", sshdContainer, "sh", "-c", "cat > /home/deploy/.ssh/authorized_keys && chown deploy:deploy /home/deploy/.ssh/authorized_keys && chmod 600 /home/deploy/.ssh/authorized_keys"], { input: publicKey });
  for (let attempt = 0; attempt < 50; attempt++) {
    try {
      const scanned = run("ssh-keyscan", ["-p", sshdPort, "-t", "ed25519", "127.0.0.1"], { stdio: ["ignore", "pipe", "ignore"] }).trim();
      if (scanned !== "") return scanned;
    } catch { /* まだ起動していない */ }
    run("sleep", ["0.2"]);
  }
  throw new Error("the fixture sshd did not answer");
}

const groupBlock = [
  "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.",
  "# Edit through the UI; lines between these markers are replaced on the next save.",
  "Include connections/production/osaka/*.conf",
  "Include connections/production/*.conf",
  "Include connections/staging/*.conf",
  "Include connections/home-lab/*.conf",
  "Include groups.sshc.conf",
  "# <<< sshc groups",
].join("\n");

function hostBlock(alias, lines) {
  return [`Host ${alias}`, ...lines.map((line) => `\t${line}`), ""].join("\n");
}

function buildHome() {
  const home = mkdtempSync(join(scratch, "ux-home-"));
  const ssh = join(home, ".ssh");
  for (const directory of ["connections/production/osaka", "connections/staging", "connections/home-lab", "conf.d"]) {
    mkdirSync(join(ssh, directory), { recursive: true, mode: 0o700 });
  }
  // 撮影のためだけの鍵。パスフレーズ付きの鍵の値は使い捨てで、撮影が終われば HOME ごと消す。
  run("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-C", "deploy@workstation", "-f", join(ssh, "id_ed25519_deploy")]);
  run("ssh-keygen", ["-q", "-t", "ed25519", "-N", "fixture passphrase", "-C", "github@workstation", "-f", join(ssh, "id_ed25519_github")]);
  run("ssh-keygen", ["-q", "-t", "rsa", "-b", "3072", "-N", "", "-C", "legacy-nas", "-f", join(ssh, "id_rsa_legacy")]);

  writeFileSync(join(ssh, "config"), [
    groupBlock,
    "# 手で管理している設定。2019 年から。",
    "",
    hostBlock("web-01", ["HostName 127.0.0.1", `Port ${sshdPort}`, "User deploy", "IdentityFile ~/.ssh/id_ed25519_deploy"]),
    hostBlock("bastion", ["HostName 203.0.113.10", "User ops", "Port 2222"]),
    hostBlock("github.com", ["User git", "IdentityFile ~/.ssh/id_ed25519_github"]),
    hostBlock("Host *", []).replace("Host Host *", "Host *") + "\tServerAliveInterval 30\n",
  ].join("\n"), { mode: 0o600 });
  writeFileSync(join(ssh, "connections/production/web.conf"), [
    hostBlock("web-02", ["HostName 127.0.0.1", `Port ${sshdPort}`, "User deploy", "IdentityFile ~/.ssh/id_ed25519_deploy"]),
    hostBlock("web-03", ["HostName 198.51.100.13", "User deploy", "ProxyJump bastion"]),
    hostBlock("web-04", ["HostName 198.51.100.14", "User deploy", "ProxyJump bastion"]),
    hostBlock("db-primary", ["HostName 198.51.100.20", "User postgres", "ProxyJump bastion", "LocalForward 15432 127.0.0.1:5432"]),
    hostBlock("db-replica", ["HostName 198.51.100.21", "User postgres", "ProxyJump bastion"]),
    hostBlock("analytics-batch-worker-very-long-hostname-01", ["HostName 198.51.100.40", "User batch", "ProxyJump bastion"]),
  ].join("\n"), { mode: 0o600 });
  writeFileSync(join(ssh, "connections/production/osaka/osaka.conf"), [
    hostBlock("osaka-web-01", ["HostName 198.51.100.61", "User deploy", "ProxyJump bastion"]),
    hostBlock("osaka-web-02", ["HostName 198.51.100.62", "User deploy", "ProxyJump bastion"]),
  ].join("\n"), { mode: 0o600 });
  writeFileSync(join(ssh, "connections/staging/staging.conf"), [
    hostBlock("stg-web", ["HostName 192.0.2.30", "User deploy"]),
    hostBlock("stg-db", ["HostName 192.0.2.31", "User postgres"]),
    hostBlock("stg-worker", ["HostName 192.0.2.32", "User deploy", "RemoteForward 9000 127.0.0.1:9000"]),
  ].join("\n"), { mode: 0o600 });
  writeFileSync(join(ssh, "connections/home-lab/lab.conf"), [
    hostBlock("nas", ["HostName 192.168.10.20", "User admin", "IdentityFile ~/.ssh/id_rsa_legacy"]),
    hostBlock("raspberrypi", ["HostName 192.168.10.31", "User pi"]),
    hostBlock("router", ["HostName 192.168.10.1", "User root", "Port 2022"]),
    hostBlock("gpu-box", ["HostName 192.168.10.50", "User admin", "DynamicForward 1080"]),
  ].join("\n"), { mode: 0o600 });
  writeFileSync(join(ssh, "groups.sshc.conf"), "", { mode: 0o600 });
  writeFileSync(join(ssh, "conf.d/legacy.conf"), hostBlock("old-mail", ["HostName 203.0.113.80", "User root", "KexAlgorithms +diffie-hellman-group1-sha1"]), { mode: 0o600 });

  // ローカルのファイル（ターミナルのプレビューと SFTP の Local で見る）。
  for (const directory of ["Pictures", "projects/api-server/src", "projects/api-server/docs", "Downloads"]) {
    mkdirSync(join(home, directory), { recursive: true });
  }
  writeFileSync(join(home, "projects/api-server/README.md"), "# api-server\n\n社内向けの API サーバー。\n\n## 起動\n\n```sh\nmake run\n```\n");
  writeFileSync(join(home, "projects/api-server/src/main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n");
  writeFileSync(join(home, "projects/api-server/docs/2026-10-29-障害報告-レスポンス遅延-詳細版-最終.md"), "# 障害報告\n");
  writeFileSync(join(home, ".bashrc"), "export PS1='$ '\nexport LANG=C.UTF-8\n");
  writeFileSync(join(home, ".bash_profile"), ". ~/.bashrc\n");
  return home;
}

function copyPicturesFromServer(home) {
  run("docker", ["cp", `${sshdContainer}:/home/deploy/photos/2026-10/.`, join(home, "Pictures")]);
  run("docker", ["cp", `${sshdContainer}:/home/deploy/docs/latency-chart.png`, join(home, "projects/api-server/docs/latency-chart.png")]);
}

function writeKnownHosts(home, scanned) {
  writeFileSync(join(home, ".ssh", "known_hosts"), [
    scanned,
    "203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl",
    "198.51.100.20 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtm",
    "|1|aGFzaGVkaG9zdG5hbWVmaXh0dXI=|Zml4dHVyZWhhc2hlZHZhbHVlMDA= ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtn",
    "",
  ].join("\n"), { mode: 0o600 });
  // web-02 は web-01 と同じ sshd なので、別名でも同じ鍵を知っている形にする。
}

// ---- エンジン ----

// startEngine は sshc engine を手元で起動する。公開のイシューに貼る画像に撮るマシンのユーザー名を写さないため、
// バイナリは HOME と同じ置き場（ユーザー名を含まない場所。UX_AUDIT_SCRATCH）へ写してから動かす（About に実行ファイルのパスが出る）。
function startEngine(home) {
  const ssh = join(home, ".ssh");
  mkdirSync(join(ssh, "sshc"), { recursive: true, mode: 0o700 });
  writeFileSync(join(ssh, "sshc", "engine-settings.json"), JSON.stringify({ schemaVersion: 1, port: Number(enginePort) }) + "\n", { mode: 0o600 });
  const binDirectory = join(scratch, "bin");
  mkdirSync(binDirectory, { recursive: true });
  run("install", ["-m", "0755", binary, join(binDirectory, "sshc")]);
  const environment = { PATH: `${binDirectory}:${process.env.PATH ?? ""}`, HOME: home, TERM: "xterm-256color", LANG: "C.UTF-8", SHELL: "/bin/bash" };
  const engine = spawn(join(binDirectory, "sshc"), ["engine", "--port", enginePort], { env: environment, stdio: ["ignore", "pipe", "pipe"] });
  let log = "";
  engine.stdout.on("data", (chunk) => { log += chunk; });
  engine.stderr.on("data", (chunk) => { log += chunk; });
  waitForEnginePort();
  return { engine, environment, log: () => log };
}

// waitForEnginePort は engine が待ち受けを始めるまで待つ。先に `sshc open` を呼ぶと、engine が止まっていると
// 判断して別の engine を裏で起動し、2つが同じ HOME を取り合う（10-28 の点検の撮影でも起きた）。
function waitForEnginePort() {
  for (let attempt = 0; attempt < 300; attempt++) {
    try {
      run("bash", ["-c", `exec 3<>/dev/tcp/127.0.0.1/${enginePort}`], { stdio: "ignore" });
      return;
    } catch { /* まだ待ち受けていない */ }
    run("sleep", ["0.1"]);
  }
  throw new Error("the sshc engine in the container did not listen");
}

let engineEnvironment = {};

function openURL() {
  for (let attempt = 0; attempt < 150; attempt++) {
    try {
      const printed = run("sshc", ["open", "--no-browser"], { env: engineEnvironment, stdio: ["ignore", "pipe", "ignore"] }).trim();
      const url = printed.split(/\s+/).find((word) => word.startsWith("http://127.0.0.1"));
      if (url !== undefined) return url;
    } catch { /* まだ起動していない */ }
    run("sleep", ["0.1"]);
  }
  throw new Error("sshc open printed no URL");
}

// 撮影のたびに作って消す一時の Vault のための使い捨ての値（実在の認証情報ではない）。
const masterPassword = "an audit master password for screenshots";

// ---- フォームファクタ ----

// 2in1 の姿勢は Chromium の起動設定で決める（PointerType: coarse=2 fine=4、HoverType: none=1 hover=2）。
// context の hasTouch は pointer を必ず coarse・hover を none にするので、キーボードを付けた姿勢を作れない。
// ターミナルは WebGL を切って DOM で描かせる。ヘッドレスの Chromium で DPR を変えると、WebGL の canvas が
// 真っ黒に写る・DPR 倍に拡大されて右が切れる（2026-10-30 の撮影で確認）。e2e の設定と同じく --disable-webgl にする。
const noWebGL = "--disable-webgl";
const launchSettings = {
  default: [noWebGL],
  laptop2in1: [noWebGL, "--blink-settings=primaryPointerType=4,availablePointerTypes=6,primaryHoverType=2,availableHoverTypes=3", "--touch-events=enabled"],
  tablet2in1: [noWebGL, "--blink-settings=primaryPointerType=2,availablePointerTypes=2,primaryHoverType=1,availableHoverTypes=1", "--touch-events=enabled"],
};

const tabletUserAgent = "Mozilla/5.0 (Linux; Android 14; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const formFactors = [
  { id: "phone", label: "スマホ縦 390×844", launch: "default", touch: true, every: true,
    context: { ...devices["Pixel 7"], viewport: { width: 390, height: 844 } } },
  { id: "tablet", label: "タブレット縦 820×1180", launch: "default", touch: true, every: true,
    context: { viewport: { width: 820, height: 1180 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, userAgent: tabletUserAgent } },
  { id: "2in1", label: "2in1 ノートPCの姿勢 1368×912", launch: "laptop2in1", touch: true, every: true,
    context: { viewport: { width: 1368, height: 912 }, deviceScaleFactor: 2 } },
  { id: "desktop", label: "デスクトップ 1920×1080", launch: "default", touch: false, every: true,
    context: { viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 } },
  { id: "phone-landscape", label: "スマホ横 844×390", launch: "default", touch: true, every: false,
    context: { ...devices["Pixel 7 landscape"], viewport: { width: 844, height: 390 } } },
  { id: "tablet-landscape", label: "タブレット横 1180×820", launch: "default", touch: true, every: false,
    context: { viewport: { width: 1180, height: 820 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, userAgent: tabletUserAgent } },
  { id: "2in1-tablet", label: "2in1 タブレットの姿勢 1368×912", launch: "tablet2in1", touch: true, every: false,
    context: { viewport: { width: 1368, height: 912 }, deviceScaleFactor: 2 } },
  { id: "laptop", label: "ノートPC 1366×768", launch: "default", touch: false, every: false,
    context: { viewport: { width: 1366, height: 768 }, deviceScaleFactor: 1 } },
];

// ---- 画面の数値 ----

// measure は撮った画面の上で、使いにくさの根拠になる数値を取る。
function measure(touch) {
  const viewportWidth = innerWidth;
  const viewportHeight = innerHeight;
  const minimumTarget = touch ? 44 : 24;
  const onScreen = (element) => {
    const rectangle = element.getBoundingClientRect();
    if (rectangle.width === 0 || rectangle.height === 0) return false;
    if (rectangle.bottom <= 0 || rectangle.right <= 0 || rectangle.top >= viewportHeight || rectangle.left >= viewportWidth) return false;
    const style = getComputedStyle(element);
    if (style.visibility === "hidden" || Number(style.opacity) === 0) return false;
    return element.closest("[aria-hidden='true'],[inert]") === null;
  };
  const name = (element) => (element.getAttribute("aria-label") || element.getAttribute("title") || element.textContent || element.getAttribute("placeholder") || element.tagName).replace(/\s+/g, " ").trim().slice(0, 48);
  const targets = [...document.querySelectorAll("button, a[href], input:not([type=hidden]), select, textarea, summary, [role=button], [role=tab], [role=menuitem], [role=option], [role=switch], [role=checkbox], [role=radio]")]
    .filter((element) => !element.closest(".xterm"))
    .filter(onScreen);
  const smallTargets = targets
    .map((element) => ({ element, rectangle: element.getBoundingClientRect() }))
    .filter(({ element, rectangle }) => Math.min(rectangle.width, rectangle.height) < minimumTarget && !(element instanceof HTMLInputElement && ["checkbox", "radio"].includes(element.type) && element.closest("label") !== null))
    .map(({ element, rectangle }) => ({ name: name(element), width: Math.round(rectangle.width), height: Math.round(rectangle.height) }));
  const truncated = [...document.querySelectorAll("body *")]
    .filter((element) => {
      const style = getComputedStyle(element);
      const clipping = style.textOverflow === "ellipsis" || style.webkitLineClamp !== "none";
      return clipping && (element.scrollWidth > element.clientWidth + 1 || element.scrollHeight > element.clientHeight + 1) && onScreen(element);
    })
    .map((element) => element.textContent.replace(/\s+/g, " ").trim().slice(0, 60))
    .filter((text) => text !== "");
  const tinyText = [...document.querySelectorAll("body *")]
    .filter((element) => [...element.childNodes].some((node) => node.nodeType === Node.TEXT_NODE && node.textContent.trim() !== ""))
    .filter((element) => !element.closest(".xterm") && parseFloat(getComputedStyle(element).fontSize) < 12 && onScreen(element))
    .map((element) => `${getComputedStyle(element).fontSize} ${element.textContent.replace(/\s+/g, " ").trim().slice(0, 30)}`);
  const main = document.querySelector("main");
  const mainRectangle = main?.getBoundingClientRect();
  const scrollers = [...document.querySelectorAll("body *")].filter((element) => {
    const style = getComputedStyle(element);
    return /(auto|scroll)/.test(style.overflowY) && element.scrollHeight > element.clientHeight + 4 && onScreen(element);
  }).length;
  return {
    viewport: { width: viewportWidth, height: viewportHeight },
    media: Object.fromEntries(["(pointer: coarse)", "(hover: hover)", "(any-hover: hover)", "(any-pointer: coarse)"].map((query) => [query, matchMedia(query).matches])),
    overflowX: document.documentElement.scrollWidth > viewportWidth + 1,
    targets: targets.length,
    smallTargets,
    truncated,
    tinyText,
    main: mainRectangle === undefined ? null : { x: Math.round(mainRectangle.x), width: Math.round(mainRectangle.width), height: Math.round(mainRectangle.height) },
    scrollers,
  };
}

const manifest = [];

async function settle(page, wait = 900) {
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(wait);
}

async function capture(page, formFactor, name, options = {}) {
  const file = `${formFactor.id}--${name}.png`;
  try {
    await settle(page, options.wait);
    await page.screenshot({ path: join(out, file), fullPage: options.fullPage ?? false });
    const numbers = await page.evaluate(measure, formFactor.touch);
    manifest.push({ file, formFactor: formFactor.id, label: formFactor.label, screen: name, url: new URL(page.url()).pathname, ...numbers });
    console.log("shot", file);
  } catch (error) {
    manifest.push({ file, formFactor: formFactor.id, screen: name, error: String(error) });
    console.log("FAILED", file, String(error).split("\n")[0]);
  }
}

async function go(page, path) {
  await page.evaluate((target) => {
    window.history.pushState(null, "", target);
    window.dispatchEvent(new PopStateEvent("popstate"));
  }, path);
}

async function signIn(page) {
  const apiLog = [];
  const logResponse = (response) => {
    if (response.url().includes("/api/")) apiLog.push(`${response.status()} ${response.request().method()} ${new URL(response.url()).pathname}`);
  };
  page.on("response", logResponse);
  try {
    await signInSteps(page);
  } finally {
    page.off("response", logResponse);
    writeFileSync(join(out, `signin-api-${Date.now()}.log`), apiLog.join("\n"));
  }
}

async function signInSteps(page) {
  await page.goto(openURL());
  // Vault を解除したあとのエンジンは、新しいブラウザにパスワードを聞かない。どちらが出るかを待つ。
  const fields = page.locator('input[type="password"]');
  // サインインの後にだけ出るナビゲーション（PC はサイドバー、スマホは下タブ）。Vault 作成の画面にも別の nav がある。
  const navigation = page.locator(`nav[aria-label="${ja("shell.primaryNavigation")}"]:visible, nav[aria-label="${ja("shell.mobileNavigation")}"]:visible`).first();
  await fields.first().or(navigation).waitFor({ timeout: 20000 }).catch(async (error) => {
    await page.screenshot({ path: join(out, `signin-first-failure-${Date.now()}.png`) }).catch(() => {});
    writeFileSync(join(out, `signin-first-failure-${Date.now()}.txt`), `${page.url()}\n${await page.content().catch(() => "")}`);
    throw error;
  });
  // 欄が出た直後は、読み込みの終わりで画面が描き直され、欄が一瞬消えたり入力が消えたりする。落ち着いてから、
  // 欄があるか（Vault の作成・解除が要るか）を判断する。早く判断すると作成を飛ばし、後の手順が全部作成の画面になる。
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(1500);
  if (await fields.first().isVisible()) {
    // 押しても作成（解除）の要求が出ないことがあった（原因は特定できていない。2026-10-30）。応答を待ち、
    // 来なければもう一度入れて押す。2回とも来なければ、後の手順がロックの画面になるので止める。
    const vaultAnswer = /\/api\/v1\/passwords\/(initialise|unlock)$/;
    let answered = false;
    for (let attempt = 0; attempt < 2 && !answered; attempt++) {
      const count = await fields.count();
      for (let index = 0; index < count; index++) await fields.nth(index).fill(masterPassword);
      const answer = page.waitForResponse((response) => vaultAnswer.test(new URL(response.url()).pathname), { timeout: 15000 }).then(() => true, () => false);
      await page.locator('button[type="submit"]').click();
      answered = await answer;
    }
    if (!answered) throw new Error("creating or unlocking the vault sent no request");
    await fields.first().waitFor({ state: "hidden", timeout: 20000 });
  }
  await navigation.waitFor({ timeout: 20000 }).catch(async (error) => {
    // 何の画面で止まったかを残してから失敗させる（撮り直しの原因を調べるため）。
    await page.screenshot({ path: join(out, `signin-failure-${Date.now()}.png`) }).catch(() => {});
    throw error;
  });
  await page.waitForTimeout(1200);
}

// api は画面と同じ cookie と CSRF の値でエンジンの API を呼ぶ。値は起動時の応答から拾っておく。
function watchCSRF(page) {
  const holder = { token: "" };
  page.on("response", async (response) => {
    if (!/\/api\/v1\/session\/(bootstrap|recover|renew)$/.test(new URL(response.url()).pathname)) return;
    try {
      const body = await response.json();
      if (typeof body.csrfToken === "string") holder.token = body.csrfToken;
    } catch { /* 本文の無い応答 */ }
  });
  return holder;
}

async function api(page, csrf, method, path, body) {
  return page.evaluate(async ({ method, path, body, token }) => {
    const response = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-SSHC-CSRF": token },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    return { status: response.status, text };
  }, { method, path, body, token: csrf.token });
}

const updateStub = (route) => route.fulfill({ json: { current: "dev", available: false, canUpdate: false } });

async function newPage(browsers, formFactor, colorScheme = "dark") {
  const context = await browsers[formFactor.launch].newContext({ ...formFactor.context, locale: "ja-JP", timezoneId: "Asia/Tokyo", colorScheme });
  await context.route(/\/api\/v1\/update(?:\/check)?$/, updateStub);
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()); });
  return { context, page, errors };
}

// ---- 画面の一覧 ----

const routes = [
  ["home", "/"],
  ["menu", "/menu"],
  ["connections", "/connections"],
  ["terminal-empty", "/terminal"],
  ["files", "/files"],
  ["snippets", "/snippets"],
  ["config", "/config"],
  ["keys", "/keys"],
  ["known-hosts", "/known-hosts"],
  ["remote-keys", "/install-key"],
  ["diagnostics", "/diagnostics"],
  ["vault-passwords", "/vault/passwords"],
  ["vault-key-passphrases", "/vault/key-passphrases"],
  ["vault-otp", "/vault/otp"],
  ["settings-appearance", "/settings/appearance"],
  ["settings-engine", "/settings/engine"],
  ["settings-terminal", "/settings/terminal"],
  ["settings-sftp", "/settings/sftp"],
  ["settings-shortcuts", "/settings/shortcuts"],
  ["settings-notifications", "/settings/notifications"],
  ["settings-connections", "/settings/connections"],
  ["settings-password", "/settings/password"],
  ["sync", "/sync"],
  ["vpn", "/vpn"],
  ["history", "/history"],
  ["license", "/license"],
  ["about", "/about"],
  ["not-found", "/no-such-page"],
];

// ---- 準備（セッション・Vault の項目・スニペットを入れる） ----

async function populate(page, csrf) {
  const results = [];
  const record = async (label, promise) => { const result = await promise; results.push([label, result.status]); return result; };
  for (const [kind, name, secret] of [
    ["password", "web-01 deploy", "fixture-password-1"],
    ["password", "nas admin", "fixture-password-2"],
    ["password", "ルーターの管理画面", "fixture-password-3"],
    ["key_passphrase", "github", "fixture passphrase"],
    ["totp", "AWS root", "JBSWY3DPEHPK3PXP"],
    ["totp", "GitHub", "GEZDGNBVGY3TQOJQ"],
  ]) {
    await record(`credential ${name}`, api(page, csrf, "PUT", `/api/v1/credentials/${kind}/${encodeURIComponent(name)}`, { secret }));
  }
  for (const snippet of [
    { name: "ディスクの空きを見る", description: "df と du で大きいフォルダを探す", command: "df -h && du -sh {{path}}/* | sort -h | tail", variables: [{ name: "path", type: "string", required: true, default: "/var/log" }] },
    { name: "サービスの再起動", description: "systemd のサービスを再起動して状態を出す", command: "sudo systemctl restart {{service}} && systemctl status {{service}} --no-pager", variables: [{ name: "service", type: "string", required: true }] },
    { name: "ログを追う", description: "", command: "tail -f /var/log/syslog", variables: [] },
    { name: "Docker の掃除", description: "止まったコンテナと使っていないイメージを消す", command: "docker system prune -f", variables: [] },
  ]) {
    await record(`snippet ${snippet.name}`, api(page, csrf, "POST", "/api/v1/snippets", snippet));
  }
  const monitoring = await record("workspace 監視", api(page, csrf, "POST", "/api/v1/terminal/workspaces", { name: "監視" }));
  const monitoringId = JSON.parse(monitoring.text || "{}").id;
  await record("workspace 検証", api(page, csrf, "POST", "/api/v1/terminal/workspaces", { name: "検証" }));
  return { results, monitoringId };
}

async function openSessions(page, csrf, home, monitoringId) {
  const opened = [];
  const open = async (body, title) => {
    const response = await api(page, csrf, "POST", "/api/v1/terminal/sessions", { cols: 120, rows: 32, ...body });
    const id = JSON.parse(response.text || "{}").id ?? JSON.parse(response.text || "{}").session?.id;
    if (id !== undefined && title !== undefined) await api(page, csrf, "PUT", `/api/v1/terminal/sessions/${id}/title`, { title });
    opened.push({ title, status: response.status, id, body: response.status >= 300 ? response.text.slice(0, 300) : "" });
    return id;
  };
  await open({ kind: "shell", cwd: join(home, "projects/api-server") }, "api-server");
  await open({ kind: "ssh", alias: "web-01" }, undefined);
  await open({ kind: "shell", cwd: join(home, "Pictures") }, "画像の確認");
  if (monitoringId !== undefined) {
    await open({ kind: "ssh", alias: "web-02", workspaceId: monitoringId }, "web-02 のログ");
    await open({ kind: "ssh", alias: "db-primary", workspaceId: monitoringId }, undefined);
  }
  return opened;
}

// typeInto は表示中のターミナルにコマンドを打つ。タッチの画面では入力欄に入るので、PC の画面でだけ使う。
async function typeInto(page, lines) {
  await page.locator(".xterm-screen").first().click({ position: { x: 40, y: 40 } });
  for (const line of lines) {
    await page.keyboard.type(line, { delay: 2 });
    await page.keyboard.press("Enter");
    await page.waitForTimeout(400);
  }
}

// selectSession はサイドバーのセッションの行（名前がセッション名のボタン）を押す。スマホの組み方では先に引き出しを開く。
async function selectSession(page, title) {
  const row = page.getByRole("button", { name: title, exact: true }).first();
  // 読み直した直後は一覧がまだ来ていないことがある。PC の組み方ではサイドバーに出るまで待ち、出なければ引き出しを開く。
  const shown = await row.waitFor({ state: "visible", timeout: 15000 }).then(() => true, () => false);
  if (!shown) await openDrawer(page);
  await row.click({ timeout: 15000 }).catch(async (error) => {
    await page.screenshot({ path: join(out, `select-failure-${Date.now()}.png`) }).catch(() => {});
    throw error;
  });
  await page.waitForTimeout(1200);
}

// clickPrintedPath はターミナルに出たパスの末尾の文字を押してプレビューを開く（e2e の terminal-path-preview と同じ方法）。
// 文字の位置は xterm の DOM の行から取る。WebGL で描いていて DOM の行が無いときは false を返す。
async function clickPrintedPath(page, text, touch) {
  const row = page.locator(".xterm-rows > div").filter({ hasText: text }).last();
  if ((await row.count()) === 0) return false;
  const bounds = await row.evaluate((element, text) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
      const at = (node.textContent ?? "").lastIndexOf(text);
      if (at < 0) continue;
      const range = document.createRange();
      range.setStart(node, at + text.length - 1);
      range.setEnd(node, at + text.length);
      const rectangle = range.getBoundingClientRect();
      return { x: rectangle.x + rectangle.width / 2, y: rectangle.y + rectangle.height / 2 };
    }
    return null;
  }, text);
  if (bounds === null) return false;
  if (touch) await page.touchscreen.tap(bounds.x, bounds.y);
  else {
    await page.mouse.move(bounds.x, bounds.y);
    await page.mouse.click(bounds.x, bounds.y);
  }
  return true;
}

// openEntry は SFTP の一覧の項目を開く。スマホの組み方では1回、PC では2回押す（mobile-ux-review.md の決まり）。
async function openEntry(page, name, mobileLayout) {
  const entry = page.getByRole("main").getByText(name, { exact: true }).first();
  if (mobileLayout) await entry.click({ timeout: 8000 });
  else await entry.dblclick({ timeout: 8000 });
  await page.waitForTimeout(1200);
}

// captureConnectionDetail は Connections で web-01 を開き、編集の4つのタブを順に撮る。
async function captureConnectionDetail(page, formFactor) {
  await go(page, "/connections");
  await page.waitForTimeout(1000);
  await page.getByRole("main").getByText("web-01", { exact: true }).first().click({ timeout: 8000 });
  await capture(page, formFactor, "connection-detail-basic", { wait: 1200 });
  for (const [key, name] of [["conn.areaAdvanced", "connection-detail-advanced"], ["conn.areaSshc", "connection-detail-terminal"], ["conn.areaAnalysis", "connection-detail-effective"]]) {
    await page.getByRole("tab", { name: ja(key), exact: true }).click({ timeout: 8000 });
    await capture(page, formFactor, name, { wait: 900 });
  }
}

async function openDrawer(page) {
  const toggle = page.getByRole("button", { name: ja("shell.navigationToggle") }).first();
  if (await toggle.isVisible().catch(() => false)) await toggle.click();
  await page.waitForTimeout(700);
}

// ---- 実行 ----

const browsers = {};
const home = buildHome();
let engine;
try {
  const scanned = startSSHServer(home);
  writeKnownHosts(home, scanned.replace(/^\S+/, `[127.0.0.1]:${sshdPort}`));
  copyPicturesFromServer(home);
  engine = startEngine(home);
  engineEnvironment = engine.environment;
  for (const [key, args] of Object.entries(launchSettings)) browsers[key] = await chromium.launch({ args });

  // 1. 入口（Vault を作る前の画面）。作る前にしか出ないので、全フォームファクタを先に撮る。
  if (wants("entry")) {
    for (const formFactor of formFactors.filter((each) => each.every)) {
      const { context, page } = await newPage(browsers, formFactor);
      await page.goto(openURL());
      await page.locator('input[type="password"]').first().waitFor({ timeout: 20000 });
      await capture(page, formFactor, "entry-create-vault");
      await context.close();
    }
  }

  // 2. Vault を作り、データを入れ、PC の画面でコマンドを打っておく。
  const desktop = formFactors.find((each) => each.id === "desktop");
  const setup = await newPage(browsers, desktop);
  const csrf = watchCSRF(setup.page);
  await signIn(setup.page);
  const populated = await populate(setup.page, csrf);
  const sessions = await openSessions(setup.page, csrf, home, populated.monitoringId);
  // Home の「確認が必要」の件数のもと（診断・お知らせ・復旧待ち）を残す。画面の文に中身が出ないため。
  const overview = await api(setup.page, csrf, "GET", "/api/v1/config/overview");
  const overviewBody = JSON.parse(overview.text || "{}");
  const attention = { diagnostics: overviewBody.diagnostics, notices: overviewBody.notices, pending: overviewBody.pending };
  writeFileSync(join(out, "setup.json"), JSON.stringify({ populated, sessions, attention }, null, 2));
  await setup.page.reload();
  await setup.page.waitForTimeout(2500);
  try {
    await selectSession(setup.page, "api-server");
    await typeInto(setup.page, ["clear", "ls -F", "git log --oneline -5 2>/dev/null || echo 'not a git repository'", `printf '%s\\n' ${join(home, "Pictures/IMG_2041.jpg")} ${join(home, "projects/api-server/docs/latency-chart.png")}`]);
    await selectSession(setup.page, "画像の確認");
    await typeInto(setup.page, ["clear", "ls -F", `printf '%s\\n' ${join(home, "Pictures/IMG_2041.jpg")} ${join(home, "Pictures/server-room.png")} ${join(home, "Pictures/demo-recording.mp4")}`]);
  } catch (error) {
    console.log("typing failed", String(error).split("\n")[0]);
  }
  await setup.context.close();

  // 3. 各画面をフォームファクタごとに撮る。
  if (wants("routes")) {
    for (const formFactor of formFactors.filter((each) => each.every)) {
      const { context, page, errors } = await newPage(browsers, formFactor);
      await signIn(page);
      for (const [name, path] of routes) {
        await go(page, path);
        await capture(page, formFactor, name, { wait: name === "config" || name === "license" ? 1800 : 1000 });
      }
      writeFileSync(join(out, `${formFactor.id}--console.txt`), errors.join("\n"));
      await context.close();
    }
  }

  // 4. 重い画面（Home・Connections・Terminal・SFTP）は、追加の姿勢でも撮る。
  if (wants("routes")) {
    for (const formFactor of formFactors.filter((each) => !each.every)) {
      const { context, page } = await newPage(browsers, formFactor);
      await signIn(page);
      for (const [name, path] of routes.filter(([name]) => ["home", "menu", "connections", "files", "settings-terminal"].includes(name))) {
        await go(page, path);
        await capture(page, formFactor, name);
      }
      try {
        await selectSession(page, "api-server");
        await capture(page, formFactor, "terminal-local-session", { wait: 1500 });
      } catch (error) { console.log("session failed", formFactor.id, String(error).split("\n")[0]); }
      await context.close();
    }
  }

  // 5. ターミナルのセッションを全フォームファクタで撮る。
  if (wants("states")) {
    for (const formFactor of formFactors.filter((each) => each.every)) {
      const { context, page } = await newPage(browsers, formFactor);
      await signIn(page);
      try {
        await selectSession(page, "api-server");
        await capture(page, formFactor, "terminal-local-session", { wait: 1500 });
        await selectSession(page, "web-01");
        await capture(page, formFactor, "terminal-ssh-session", { wait: 2500 });
      } catch (error) { console.log("session failed", formFactor.id, String(error).split("\n")[0]); }
      await context.close();
    }
  }

  // 6. ライトの配色で入口以外の主な画面を撮る。
  if (wants("light")) {
    for (const formFactor of formFactors.filter((each) => ["phone", "desktop"].includes(each.id))) {
      const { context, page } = await newPage(browsers, formFactor, "light");
      await signIn(page);
      for (const [name, path] of routes.filter(([name]) => ["home", "connections", "files", "menu"].includes(name))) {
        await go(page, path);
        await capture(page, formFactor, `light-${name}`);
      }
      await context.close();
    }
  }

  // 7. 操作した状態（プレビュー、メニュー、ダイアログ、SFTP の接続後、接続先の編集、引き出し）を撮る。
  if (wants("interact")) {
    for (const formFactor of formFactors.filter((each) => each.every || each.id === "2in1-tablet")) {
      const { context, page } = await newPage(browsers, formFactor);
      await signIn(page);
      const mobileLayout = await page.evaluate(() => matchMedia("(max-width: 767px), (pointer: coarse) and (max-width: 1023px)").matches);
      const step = async (label, action) => {
        try { await action(); } catch (error) { console.log("step failed", formFactor.id, label, String(error).split("\n")[0]); }
        await page.keyboard.press("Escape").catch(() => {});
        await page.waitForTimeout(300);
      };

      await step("drawer", async () => {
        if (!mobileLayout) return;
        await openDrawer(page);
        await capture(page, formFactor, "drawer-open");
      });

      await step("preview", async () => {
        await selectSession(page, "画像の確認");
        await capture(page, formFactor, "terminal-image-session", { wait: 1500 });
        const opened = await clickPrintedPath(page, "IMG_2041.jpg", mobileLayout);
        if (!opened) throw new Error("the printed path was not found in the terminal rows");
        await page.waitForTimeout(1500);
        await capture(page, formFactor, "terminal-preview-image", { wait: 1200 });
      });

      await step("more actions", async () => {
        await selectSession(page, "api-server");
        await page.getByRole("button", { name: ja("terminal.moreActions") }).first().click({ timeout: 8000 });
        await capture(page, formFactor, "terminal-more-actions");
      });

      await step("new session", async () => {
        if (mobileLayout) await openDrawer(page);
        await page.getByRole("button", { name: ja("terminal.newSession"), exact: true }).first().click({ timeout: 8000 });
        await capture(page, formFactor, "new-session-dialog");
      });

      await step("palette", async () => {
        if (mobileLayout) return;
        await go(page, "/");
        await page.keyboard.press("Control+k");
        await page.keyboard.type("web", { delay: 20 });
        await capture(page, formFactor, "command-palette");
      });

      await step("sftp", async () => {
        await go(page, "/files");
        await page.waitForTimeout(800);
        await page.getByRole("button", { name: ja("sftp.chooseHost"), exact: true }).last().click({ timeout: 8000 });
        await capture(page, formFactor, "sftp-host-picker");
        await page.getByRole("dialog").getByText("web-01", { exact: true }).first().click();
        await page.waitForTimeout(2500);
        await capture(page, formFactor, "sftp-remote-home", { wait: 1500 });
        await openEntry(page, "photos", mobileLayout);
        await openEntry(page, "2026-10", mobileLayout);
        await capture(page, formFactor, "sftp-remote-photos", { wait: 1200 });
        await openEntry(page, "IMG_2041.jpg", mobileLayout);
        await page.waitForTimeout(2000);
        await capture(page, formFactor, "sftp-preview-image", { wait: 1200 });
      });

      await step("connection detail", () => captureConnectionDetail(page, formFactor));

      await step("new connection", async () => {
        await go(page, "/connections");
        await page.waitForTimeout(800);
        await page.getByRole("button", { name: /接続先を追加/ }).first().click({ timeout: 8000 });
        await capture(page, formFactor, "connection-new", { wait: 1200 });
      });

      await context.close();
    }
  }

  // 9. 長い行の折り返し: 画面から普通に開いたセッションで長い行を打ち、幅の違うブラウザで同じセッションを開いて比べる。
  // 印の付いた文字列（_00_abcdefghij…）にして、どこが欠けた・置き換わったかを読めるようにする。
  if (wants("reflow")) {
    const desktopFactor = formFactors.find((each) => each.id === "desktop");
    const typing = await newPage(browsers, desktopFactor);
    const csrf = watchCSRF(typing.page);
    await signIn(typing.page);
    await typing.page.getByRole("button", { name: ja("terminal.newSession"), exact: true }).first().click();
    await typing.page.getByRole("dialog").getByRole("button", { name: /^Local shell/ }).first().click();
    await typing.page.waitForTimeout(2500);
    const listed = JSON.parse((await api(typing.page, csrf, "GET", "/api/v1/terminal/sessions")).text || "{}");
    const fresh = (listed.sessions ?? []).filter((session) => session.kind === "shell").sort((a, b) => b.startedAt.localeCompare(a.startedAt))[0];
    await api(typing.page, csrf, "PUT", `/api/v1/terminal/sessions/${fresh.id}/title`, { title: "折り返しの確認" });
    await typing.page.waitForTimeout(800);
    const marked = Array.from({ length: 24 }, (_, index) => `_${String(index).padStart(2, "0")}_abcdefghij`).join("");
    await typeInto(typing.page, ["clear", `echo ${marked}`, `echo ${marked}`]);
    await capture(typing.page, desktopFactor, "reflow-1-typed", { wait: 1200 });
    for (const id of ["phone", "laptop", "tablet"]) {
      const formFactor = formFactors.find((each) => each.id === id);
      const other = await newPage(browsers, formFactor);
      await signIn(other.page);
      try {
        await selectSession(other.page, "折り返しの確認");
        await capture(other.page, formFactor, "reflow-2-other", { wait: 1500 });
      } catch (error) { console.log("reflow failed", id, String(error).split("\n")[0]); }
      await other.context.close();
    }
    await typing.page.waitForTimeout(1000);
    await capture(typing.page, desktopFactor, "reflow-3-typed-after", { wait: 1200 });
    await typing.context.close();
  }

  // 10. 2in1 のノートPCの姿勢（pointer: fine、any-hover: hover）で指で触る。「…」はマウスを載せたときだけ出る
  // （hover-reveal）ので、指で触ったときにメニューへ届くかを見る。タップは CDP の touch で送る（hasTouch は姿勢を変えてしまう）。
  if (wants("touch2in1")) {
    const formFactor = formFactors.find((each) => each.id === "2in1");
    const { context, page } = await newPage(browsers, formFactor);
    await signIn(page);
    const cdp = await context.newCDPSession(page);
    const touch = async (locator, holdMillis = 60) => {
      const box = await locator.boundingBox();
      if (box === null) throw new Error("nothing to touch");
      const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
      await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [point] });
      await page.waitForTimeout(holdMillis);
      await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      await page.waitForTimeout(900);
    };
    const opacityOf = (locator) => locator.evaluate((element) => getComputedStyle(element).opacity).catch(() => "missing");
    const report = {};
    try {
      await go(page, "/");
      await page.waitForTimeout(1200);
      const card = page.getByRole("main").getByText("db-replica", { exact: true }).first();
      const cardMenu = page.getByRole("button", { name: /db-replicaの操作/ }).first();
      report.homeMenuBefore = await opacityOf(cardMenu);
      await touch(card);
      report.homeMenuAfterTap = await opacityOf(cardMenu);
      report.urlAfterHomeTap = new URL(page.url()).pathname;
      await capture(page, formFactor, "touch-home-tap");
      await go(page, "/");
      await page.waitForTimeout(1000);
      await touch(card, 800);
      report.homeMenuAfterLongPress = await opacityOf(cardMenu);
      await capture(page, formFactor, "touch-home-long-press");
      await page.keyboard.press("Escape");
      const row = page.getByRole("button", { name: "web-01", exact: true }).first();
      const rowMenu = page.getByRole("button", { name: ja("terminal.rowMenu").replace("{title}", "web-01") }).first();
      report.rowMenuBefore = await opacityOf(rowMenu);
      await touch(row);
      report.rowMenuAfterTap = await opacityOf(rowMenu);
      await touch(row, 800);
      report.rowMenuAfterLongPress = await opacityOf(rowMenu);
      await capture(page, formFactor, "touch-sidebar-long-press");
    } catch (error) { report.error = String(error).split("\n")[0]; }
    writeFileSync(join(out, "touch2in1.json"), JSON.stringify(report, null, 2));
    await context.close();
  }

  // 11. PC の組み方の接続先の編集（interact で撮れなかったときに、この段だけ流す）。
  if (wants("connections")) {
    for (const formFactor of formFactors.filter((each) => ["2in1", "desktop", "2in1-tablet"].includes(each.id))) {
      const { context, page } = await newPage(browsers, formFactor);
      await signIn(page);
      try { await captureConnectionDetail(page, formFactor); } catch (error) { console.log("connections failed", formFactor.id, String(error).split("\n")[0]); }
      await context.close();
    }
  }

  // 12. 試作: 案ごとの CSS や操作を今の画面に差し込んで撮る。案の定義は UX_OPTIONS のモジュール（default で
  // { id, formFactor, open(page, helpers), options: [{ id, css?, apply?(page, helpers), context?, launch? }] } の配列）。
  // 案ごとに新しいページで開き直すので、前の案の差し込みは残らない。
  if (stages.has("options")) {
    const specs = (await import(resolve(process.env.UX_OPTIONS))).default;
    const only = process.env.UX_OPTIONS_ONLY === undefined ? null : new Set(process.env.UX_OPTIONS_ONLY.split(","));
    for (const spec of specs.filter((each) => only === null || only.has(each.id))) {
      const formFactor = formFactors.find((each) => each.id === spec.formFactor);
      for (const option of spec.options) {
        const shown = { ...formFactor, launch: option.launch ?? formFactor.launch, context: { ...formFactor.context, ...option.context } };
        const { context, page } = await newPage(browsers, shown);
        const csrf = watchCSRF(page);
        await signIn(page);
        const mobileLayout = await page.evaluate(() => matchMedia("(max-width: 767px), (pointer: coarse) and (max-width: 1023px)").matches);
        const helpers = { go, ja, home, selectSession, clickPrintedPath, openEntry, openDrawer, settle, mobileLayout, signIn, api: (method, path, body) => api(page, csrf, method, path, body) };
        try {
          await spec.open(page, helpers);
          if (option.css !== undefined) await page.addStyleTag({ content: option.css });
          if (option.apply !== undefined) await option.apply(page, helpers);
          await capture(page, shown, `option-${spec.id}-${option.id}`, { wait: option.wait ?? 800 });
        } catch (error) { console.log("option failed", spec.id, option.id, String(error).split("\n")[0]); }
        await context.close();
      }
    }
  }

  // 8. Vault をロックした画面（ロック解除の入口）を撮る。
  if (wants("lock")) {
    const desktopFactor = formFactors.find((each) => each.id === "desktop");
    const locker = await newPage(browsers, desktopFactor);
    const csrf = watchCSRF(locker.page);
    await signIn(locker.page);
    const locked = await api(locker.page, csrf, "POST", "/api/v1/passwords/lock", {});
    console.log("lock", locked.status);
    await locker.context.close();
    for (const formFactor of formFactors.filter((each) => each.every)) {
      const { context, page } = await newPage(browsers, formFactor);
      await page.goto(openURL());
      await page.locator('input[type="password"]').first().waitFor({ timeout: 20000 });
      await capture(page, formFactor, "entry-unlock-vault");
      await context.close();
    }
  }
} finally {
  writeFileSync(join(out, `manifest-${[...stages].join("-")}.json`), JSON.stringify(manifest, null, 2));
  for (const browser of Object.values(browsers)) await browser.close().catch(() => {});
  if (engine !== undefined) {
    engine.engine.kill("SIGTERM");
    writeFileSync(join(out, "engine.log"), engine.log());
  }
  if (process.env.UX_AUDIT_KEEP !== "1") {
    try { run("docker", ["rm", "-f", sshdContainer], { stdio: "ignore" }); } catch { /* 消えている */ }
    rmSync(home, { recursive: true, force: true });
  }
}

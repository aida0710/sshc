import { expect, openApplication, openSection, test } from "./support/environment";

// 埋め込みターミナルの end-to-end。
//
// ここが駆動するのは実バイナリであり、開かれるのは本物の PTY である。ローカル
// シェルは一時 HOME の中で完結するので、このスイートはリモートホストへ一度も
// 触れない。
//
// CSP は緩めていない。xterm.js の配布物には innerHTML も document.write も
// new Function も無いので、`require-trusted-types-for 'script'` に触れる経路が
// そもそも存在しない。それを毎回確かめるために、どの spec も違反を監視する。
function watchForPolicyViolations(page: import("@playwright/test").Page): string[] {
  const violations: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (/Content Security Policy|Trusted Type/i.test(text)) violations.push(text);
  });
  page.on("pageerror", (error) => {
    if (/Trusted Type|Content Security Policy/i.test(error.message)) violations.push(error.message);
  });
  return violations;
}

// 打鍵は xterm の隠しテキストエリアへ届かなければならない。パネルのボタンを
// 押した直後は焦点がそこにあるので、まず端末へ焦点を移し、シェルがプロンプトを
// 描くのを待ってから打つ。
async function typeIntoConsole(page: import("@playwright/test").Page, line: string) {
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  // プロンプトが出るまでは、打った文字はどこにも解釈されない。
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
  return screen;
}

async function openConsolePanel(page: import("@playwright/test").Page) {
  await openSection(page, "Connections");
  // トグルは接続を開いていなくても出る。コンソールの面には常に開くべきものが
  // あるからだ。すでに開いていれば押さない——押せば閉じてしまう。
  const toggle = page.getByRole("button", { name: /^Show |^Hide / });
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) !== "true") await toggle.click();
  const panel = page.getByRole("complementary");
  await expect(panel.getByRole("button", { name: "Local shell" })).toBeVisible();
  return panel;
}

test("opens a local shell, runs a command and shows its output", async ({ page, installation }) => {
  const violations = watchForPolicyViolations(page);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();

  // 端末は主画面に出る。xterm.js が描いた行がそこに現れる。
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();

  // 打鍵はそのまま PTY へ渡る。シェルはプロンプトを描いてから答える。
  await typeIntoConsole(page, "echo embedded-terminal-canary");

  await expect(screen).toContainText("embedded-terminal-canary", { timeout: 20_000 });
  expect(violations).toEqual([]);
});

test("keeps the session and replays its scrollback after a reload", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const screen = await typeIntoConsole(page, "echo survives-a-reload");
  await expect(screen).toContainText("survives-a-reload", { timeout: 20_000 });

  // PTY は常駐プロセス側で存続する。タブを閉じてもリロードしてもセッションは
  // 生きており、繋ぎ直すとスクロールバックが先に再生される。
  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();
  const reopened = await openConsolePanel(page);
  const row = reopened.getByRole("button", { name: /^zsh|^bash|^sh / }).first();
  await expect(row).toBeVisible();
  await row.click();

  await expect(page.getByRole("region", { name: /^Console for / }))
    .toContainText("survives-a-reload", { timeout: 20_000 });
});

test("refuses to open more consoles than the configured limit", async ({ page, installation }) => {
  // 上限は metadata が運ぶ。2 本まで開ける状態にしてから、3 本目が拒否される
  // ことを見る。黙って古いセッションを閉じることはしない。
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({ schemaVersion: 3, embeddedTerminal: { maxSessions: 2, scrollbackBytes: 16384 } }),
  );
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const openShell = panel.getByRole("button", { name: "Local shell" });

  await openShell.click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await openShell.click();

  // 二本開いた時点で入口は閉じ、その理由が書かれる。
  await expect(openShell).toBeDisabled();
  await expect(panel).toContainText("limit of 2 open consoles");
});

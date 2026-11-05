import type { Page } from "@playwright/test";
import { resolve } from "node:path";
import { changeDisplayLanguage, expect, openApplication, openLocalShell, test } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

// The two feature cards at the top of the documentation's home page show these screens. A card is half the page's
// width, so the screen is captured smaller than a desktop window for the sidebar's text to stay readable there, and
// at twice the pixels for high-density displays.
const cardViewport = { width: 860, height: 484 };
const cardScale = 2;

test.use({ viewport: cardViewport, deviceScaleFactor: cardScale });

const claudeCodePermissionScreen = resolve("e2e/fixtures/claude-code-permission.txt");

const reportProgramStatus = (fields: string) => `printf '\\033]7501;${fields}\\033\\\\'`;
const base64Of = (text: string) => Buffer.from(text).toString("base64");
// A title is typed as base64 because the keyboard cannot type Japanese into the shell.
const decoded = (text: string) => `"$(printf %s ${base64Of(text)} | base64 -d)"`;
const setTitle = (title: string) => `printf '\\033]0;%s\\a' ${decoded(title)}`;
// showClaudeCodeQuestion draws Claude Code asking for permission, after the prompt it was given, and leaves the shell
// waiting so that its prompt stays off the screen.
const showClaudeCodeQuestion = (prompt: string) => `PS1=''; clear; printf '%s\\n\\n' ${decoded(`❯ ${prompt}`)}; cat '${claudeCodePermissionScreen}'; read -r _`;

async function typeInTerminal(page: Page, line: string) {
  await terminalKeyboard(page).focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
}

async function openShell(page: Page) {
  const before = await sessionRows(page).count();
  await openLocalShell(page);
  await expect(sessionRows(page)).toHaveCount(before + 1);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toContainText(/[$#%>]/, { timeout: 20_000 });
}

function sessionRows(page: Page) {
  return page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" }).getByRole("listitem");
}

async function capture(page: Page, name: string) {
  const directory = process.env.SSHC_VISUAL_DIR;
  if (directory === undefined) return;
  await page.mouse.move(0, 0);
  await page.screenshot({ path: `${directory}/${name}.png`, animations: "disabled" });
}

// The notice that asks to allow notifications would cover the terminal in the capture.
async function allowNotifications(page: Page) {
  await page.addInitScript(() => {
    class AllowedNotification extends EventTarget {
      static permission = "granted";
      static requestPermission = async () => "granted";
      close() {}
    }
    Object.assign(window, { Notification: AllowedNotification });
  });
}

// What the programs say in the capture, in the language it is captured in. The session list's own words come from the
// display language.
const programStatusCopy = {
  ja: {
    fixingTests: "テストの失敗を直す",
    prompt: "テストを実行して、失敗があれば直して",
    updatingDocs: "READMEの更新",
    addingTests: "テストの追加",
    finished: "テストを12件追加しました",
    permissionLabel: "許可待ち",
    finishedLabel: "完了",
    workingLabel: "作業中",
  },
  en: {
    fixingTests: "Fix failing tests",
    prompt: "Run the tests and fix any failures",
    updatingDocs: "Update the README",
    addingTests: "Add tests",
    finished: "Added 12 tests",
    permissionLabel: "Needs permission",
    finishedLabel: "Done",
    workingLabel: "Working",
  },
} as const;

for (const locale of ["ja", "en"] as const) {
  const copy = programStatusCopy[locale];

  // Claude Code reports these states itself; the shell's printf stands in for it here, since the suite has no signed-in
  // Claude Code. The session with the question is on screen, as a person would have it.
  test(`shows what each program is doing in the session list (${locale})`, async ({ page, installation }) => {
    test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
    await allowNotifications(page);
    await openApplication(page, installation);

    await openShell(page);
    await typeInTerminal(page, `${setTitle(copy.fixingTests)}; ${reportProgramStatus(`state=blocked:kind=permission:app=claude-code:msg=${base64Of("approve Bash: npm test")}`)}; ${showClaudeCodeQuestion(copy.prompt)}`);
    await expect(sessionRows(page).first()).toContainText("Needs permission", { timeout: 15_000 });

    await openShell(page);
    await typeInTerminal(page, `${setTitle(copy.updatingDocs)}; ${reportProgramStatus("state=working:app=claude-code")}; read -r _`);

    // The result comes once another session is on screen: a result the person has seen is not shown.
    await openShell(page);
    await typeInTerminal(page, `${setTitle(copy.addingTests)}; sleep 4; ${reportProgramStatus(`state=done:app=claude-code:msg=${base64Of(copy.finished)}`)}; read -r _`);
    await sessionRows(page).first().getByRole("button").first().click();
    await expect(sessionRows(page).nth(2)).toContainText("Done", { timeout: 15_000 });

    await changeDisplayLanguage(page, locale);
    const sidebar = page.getByRole("navigation").first();
    await expect(sidebar).toContainText(copy.permissionLabel);
    await expect(sidebar).toContainText(copy.workingLabel);
    await expect(sidebar).toContainText(`${copy.finishedLabel} · ${copy.finished}`);
    await expect(page.getByRole("region").filter({ hasText: "Do you want to proceed?" })).toBeVisible();
    await capture(page, `program-status-desktop-${locale}`);
  });
}

const remoteEngine = "gpu-server";
const otherRemoteEngine = "build-server";

// serveEngineFromThisMachine makes the browser see an engine on an SSH host, which the suite does not have, by sending
// its terminal requests to this machine's engine: the two have the same API. The requests are redirected in the page
// because the engine accepts only the browser's own same-origin requests.
async function serveEngineFromThisMachine(page: Page) {
  await page.addInitScript(({ alias }) => {
    const toThisMachine = (url: string) => url.replace(`/engines/${alias}/`, "/");
    const RealWebSocket = window.WebSocket;
    window.WebSocket = class extends RealWebSocket {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(toThisMachine(String(url)), protocols);
      }
    };
    const realFetch = window.fetch.bind(window);
    window.fetch = (input, init) => (input instanceof Request ? realFetch(new Request(toThisMachine(input.url), input), init) : realFetch(toThisMachine(String(input)), init));
  }, { alias: remoteEngine });
  await page.route("**/api/v1/engines", async (route) => {
    await route.fulfill({ json: { engines: [{ alias: otherRemoteEngine, connected: true }, { alias: remoteEngine, connected: true }] } });
  });
  await page.route(`**/api/v1/engines/${otherRemoteEngine}/**`, async (route) => {
    await route.fulfill({ json: { sessions: [], maxSessions: 50 } });
  });
}

const remoteSessionCopy = {
  ja: { engineList: "実行中のsshcエンジン", title: "テストの失敗を直す", prompt: "テストを実行して、失敗があれば直して" },
  en: { engineList: "Engines", title: "Fix failing tests", prompt: "Run the tests and fix any failures" },
} as const;

for (const locale of ["ja", "en"] as const) {
  const copy = remoteSessionCopy[locale];

  test(`runs the terminals on the engine chosen in the remote session menu (${locale})`, async ({ page, installation }) => {
    test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
    await allowNotifications(page);
    await serveEngineFromThisMachine(page);
    await openApplication(page, installation);

    const navigation = page.getByRole("navigation", { name: "Primary" });
    const switcher = navigation.getByRole("button", { name: /^Choose the remote session engine/ });
    await switcher.click();
    // A row is named by the engine and then its state (Connected).
    await navigation.getByRole("list", { name: "Engines" }).getByRole("button", { name: new RegExp(`^${remoteEngine}`) }).click();
    await expect(switcher).toContainText(remoteEngine);

    await navigation.getByRole("button", { name: "New session", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "New session" });
    await dialog.getByRole("button", { name: /^Local shell/ }).first().click();
    await expect(dialog).toBeHidden();
    const terminal = page.getByRole("region", { name: /^Terminal for / });
    await expect(terminal).toContainText(/[$#%>]/, { timeout: 20_000 });
    await expect(terminal).toContainText(`on ${remoteEngine}`);
    await typeInTerminal(page, `${setTitle(copy.title)}; ${reportProgramStatus(`state=blocked:kind=permission:app=claude-code:msg=${base64Of("approve Bash: npm test")}`)}; ${showClaudeCodeQuestion(copy.prompt)}`);
    await expect(sessionRows(page).first()).toContainText("Needs permission", { timeout: 15_000 });
    await expect(terminal).toContainText("Do you want to proceed?");

    await changeDisplayLanguage(page, locale);
    await page.getByRole("navigation").first().getByRole("button", { expanded: false }).filter({ hasText: remoteEngine }).click();
    await expect(page.getByRole("list", { name: copy.engineList })).toBeVisible();
    await capture(page, `remote-session-desktop-${locale}`);
  });
}

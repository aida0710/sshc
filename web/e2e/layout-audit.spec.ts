import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { messages } from "../src/i18n/messages";
import type { MessageKey } from "../src/i18n/messages";
import {
  changeDisplayLanguage,
  expect,
  masterPassword,
  openApplication,
  test,
} from "./support/environment";
import { behaveAsAndroidApp } from "./support/androidApp";
import { collectLayoutDefects, describeLayoutDefect, type LayoutDefect } from "./support/layoutDefects";
import {
  addWorstCaseRecords,
  answerAbout,
  answerConfiguredSync,
  answerInterruptedChange,
  answerManyMachines,
  answerUpdateAvailable,
  answerVPNWithoutDocker,
  answerWorstCaseHistory,
  answerWorstCaseScan,
  answerWorstCaseVPN,
  longConnectionName,
  longGroupName,
  writeEmptyHome,
  writeWorstCaseHome,
} from "./support/worstCaseData";

// Every page, in Japanese, at the sizes a person uses, over the worst data and over none. A page that breaks here is
// cut off or out of reach for someone, though every other test still finds its buttons.

// A step that cannot find what it presses should fail soon, not when the long test as a whole runs out.
test.use({ actionTimeout: 10_000 });

const windowSizes = [
  { width: 1280, height: 800 },
  { width: 1024, height: 700 },
  { width: 412, height: 842 },
  { width: 360, height: 640 },
] as const;

type WindowSize = (typeof windowSizes)[number];

// SSHC_LAYOUT_AUDIT_DIR keeps a picture of each screen and what was found on it, for reading by eye: the check cannot
// see that a row has a different shape from its neighbours. Nothing is kept without it, as in the rest of the suite.
const keptPicturesDirectory = process.env.SSHC_LAYOUT_AUDIT_DIR;
const themes = keptPicturesDirectory === undefined ? (["dark"] as const) : (["dark", "light"] as const);

// A page has settled once no request has been open for this long. A request open longer than firstAnswersMs is one that
// waits for news rather than for an answer, and is not waited for.
const quietNetworkMs = 200;
const firstAnswersMs = 3_000;
// Panels slide and fade in for about this long; a picture taken earlier catches them on the way.
const animationMs = 350;
// The tallest picture of a page that scrolls. A longer page is the same rows again.
const tallestPicturePx = 3_200;
// More than the 80px by which useAppViewport tells a new window from an old one.
const widthThatForgetsTheHeightPx = 100;

// Below this width a window is laid out as a phone.
const narrowestPCLayoutWidth = 768;
// Below this width Connections keeps its groups in a sheet rather than a column beside the list.
const narrowestGroupColumnWidth = 1280;

function ja(key: MessageKey): string {
  return messages.ja[key];
}

async function goTo(page: Page, path: string): Promise<void> {
  await page.evaluate((target) => {
    window.history.pushState(null, "", target);
    window.dispatchEvent(new PopStateEvent("popstate"));
  }, path);
}

// When each of a page's open requests started, for settle to wait on.
const openRequests = new WeakMap<Page, Map<unknown, number>>();

function watchRequests(page: Page): void {
  const open = new Map<unknown, number>();
  openRequests.set(page, open);
  page.on("request", (request) => open.set(request, Date.now()));
  page.on("requestfinished", (request) => open.delete(request));
  page.on("requestfailed", (request) => open.delete(request));
}

async function settle(page: Page): Promise<void> {
  const open = openRequests.get(page) ?? new Map<unknown, number>();
  const awaitingAnswer = () => [...open.values()].some((startedAt) => Date.now() - startedAt < firstAnswersMs);
  let quietSince = Date.now();
  while (Date.now() - quietSince < quietNetworkMs) {
    if (awaitingAnswer()) quietSince = Date.now();
    await page.waitForTimeout(quietNetworkMs / 4);
  }
  await page.waitForTimeout(animationMs);
}

// A record of one walk over the screens: what was found, and where the pictures went.
class Walk {
  readonly found: { screen: string; defects: LayoutDefect[] }[] = [];

  constructor(
    private readonly page: Page,
    private readonly label: string,
    private readonly size: WindowSize,
  ) {}

  async look(screen: string): Promise<void> {
    await settle(this.page);
    await this.keepPictures(screen);
    this.found.push({ screen, defects: await collectLayoutDefects(this.page) });
  }

  private async keepPictures(screen: string): Promise<void> {
    if (keptPicturesDirectory === undefined) return;
    const directory = join(keptPicturesDirectory, this.label);
    await mkdir(directory, { recursive: true });
    const name = `${screen}--${this.size.width}x${this.size.height}`;
    await this.page.screenshot({ path: join(directory, `${name}.png`) });
    // The application scrolls inside its own panes, so a full-page picture would show one window's worth. A tall window
    // shows what a person reaches by scrolling.
    const contentHeight = await this.page.evaluate(() => Math.max(
      ...[...document.querySelectorAll("main, main *, [role='dialog'], [role='dialog'] *")].map((element) => {
        const style = getComputedStyle(element);
        return style.overflowY === "auto" || style.overflowY === "scroll" ? element.scrollHeight - element.clientHeight : 0;
      }),
      0,
    ));
    if (contentHeight <= 8) return;
    await this.page.setViewportSize({ width: this.size.width, height: Math.min(tallestPicturePx, this.size.height + contentHeight) });
    await this.page.waitForTimeout(animationMs);
    await this.page.screenshot({ path: join(directory, `${name}--tall.png`) });
    // The page takes a window that gets shorter for a keyboard opening (useAppViewport), and it would then move what is
    // on the screen each time a field takes the focus. A window that also changes its width is a new window to it.
    await this.page.setViewportSize({ width: this.size.width + widthThatForgetsTheHeightPx, height: this.size.height });
    await this.page.waitForTimeout(animationMs);
    await this.page.setViewportSize(this.size);
    await this.page.waitForTimeout(animationMs);
  }

  async finish(): Promise<void> {
    if (keptPicturesDirectory !== undefined) {
      await mkdir(keptPicturesDirectory, { recursive: true });
      await writeFile(join(keptPicturesDirectory, `${this.label}--${this.size.width}x${this.size.height}.json`), JSON.stringify(this.found, null, 1));
      return;
    }
    const lines = this.found.flatMap(({ screen, defects }) => defects.map((defect) => `${screen}: ${describeLayoutDefect(defect)}`));
    expect(lines).toEqual([]);
  }
}

const pageRoutes: readonly (readonly [string, string])[] = [
  ["home", "/"],
  ["menu", "/menu"],
  ["connections", "/connections"],
  ["snippets", "/snippets"],
  ["config", "/config"],
  ["keys", "/keys"],
  ["known-hosts", "/known-hosts"],
  ["install-key", "/install-key"],
  ["diagnostics", "/diagnostics"],
  ["vault-passwords", "/vault/passwords"],
  ["vault-key-passphrases", "/vault/key-passphrases"],
  ["vault-otp", "/vault/otp"],
  ["settings-general", "/settings/general"],
  ["settings-appearance", "/settings/appearance"],
  ["settings-terminal", "/settings/terminal"],
  ["settings-sftp", "/settings/sftp"],
  ["settings-shortcuts", "/settings/shortcuts"],
  ["settings-keybar", "/settings/keybar"],
  ["settings-notifications", "/settings/notifications"],
  ["settings-vault", "/settings/vault"],
  ["settings-machines", "/settings/machines"],
  ["settings-connections", "/settings/connections"],
  ["sync", "/sync"],
  ["vpn", "/vpn"],
  ["history", "/history"],
  ["license", "/license"],
  ["about", "/about"],
  ["not-found", "/no-such-page"],
];

async function lookAtEveryPage(page: Page, walk: Walk): Promise<void> {
  for (const [screen, path] of pageRoutes) {
    await goTo(page, path);
    await walk.look(screen);
  }
}

// openConnection opens the connection whose name is given, from the list of Connections.
async function openConnection(page: Page, alias: string): Promise<void> {
  await goTo(page, "/connections");
  await settle(page);
  await page.getByRole("main").getByText(alias, { exact: true }).first().click();
}

async function lookAtConnectionEditor(page: Page, walk: Walk): Promise<void> {
  await openConnection(page, longConnectionName);
  await walk.look("connection-basic");
  for (const [tab, screen] of [["conn.areaAdvanced", "connection-advanced"], ["conn.areaSshc", "connection-terminal"], ["conn.areaAnalysis", "connection-effective"]] as const) {
    await page.getByRole("tab", { name: ja(tab), exact: true }).click();
    await walk.look(screen);
  }
  await page.getByRole("button", { name: ja("conn.analysisRun"), exact: true }).click();
  await expect(page.getByRole("table", { name: ja("conn.analysisSources") })).toBeVisible();
  await walk.look("connection-effective-sources");
}

async function lookAtConnectionForms(page: Page, walk: Walk, size: WindowSize): Promise<void> {
  await goTo(page, "/connections");
  await settle(page);
  // A pointer chooses several connections with Ctrl+click; only a phone has a mode for it.
  if (size.width < narrowestPCLayoutWidth) {
    await page.getByRole("button", { name: ja("tree.select"), exact: true }).click();
    await page.getByRole("main").getByRole("button", { name: longConnectionName }).first().click();
    await walk.look("connection-selection");
    await page.getByRole("button", { name: ja("tree.selectDone"), exact: true }).click();
  }
  await page.getByRole("button", { name: ja("conn.new"), exact: true }).first().click();
  await walk.look("connection-new");
  await page.getByRole("button", { name: ja("conn.cancelCreate"), exact: true }).click();
}

async function lookAtGroups(page: Page, walk: Walk, size: WindowSize): Promise<void> {
  await goTo(page, "/connections");
  await settle(page);
  const actions = ja("tree.groupActions").replace("{name}", longGroupName);
  if (size.width < narrowestGroupColumnWidth) {
    await page.getByRole("main").getByRole("button", { name: ja("tree.groupButton").replace("{name}", ja("tree.allConnections")) }).click();
    await walk.look("group-sheet");
    await page.getByRole("button", { name: actions }).click();
  } else {
    const groups = page.getByRole("navigation", { name: ja("tree.byGroups") });
    await groups.getByRole("button", { name: ja("tree.newGroup"), exact: true }).click();
    await walk.look("group-new");
    await page.keyboard.press("Escape");
    await groups.getByRole("button", { name: longGroupName, exact: true }).hover();
    await groups.getByRole("button", { name: actions }).click();
  }
  await walk.look("group-menu");
  await page.getByRole("menuitem", { name: ja("tree.editGroup") }).click();
  await walk.look("group-editor");
}

async function lookAtPageForms(page: Page, walk: Walk): Promise<void> {
  await goTo(page, "/keys");
  await settle(page);
  await page.getByRole("button", { name: /id_ed25519_customer_acme/ }).first().click();
  await walk.look("keys-detail");
  // Narrower than a PC's window the details cover the page, and they stay until closed.
  const closeDetails = page.getByRole("button", { name: ja("shell.inspectorHideNamed").replace("{label}", ja("keys.inspectorLabel")) }).first();
  if (await closeDetails.isVisible()) await closeDetails.click();
  await page.getByRole("button", { name: ja("keys.createHeading"), exact: true }).first().click();
  await walk.look("keys-create");

  await goTo(page, "/known-hosts");
  await settle(page);
  await page.getByRole("main").getByRole("textbox").first().fill("example.com");
  await page.getByRole("button", { name: ja("kh.scan"), exact: true }).click();
  await expect(page.getByRole("button", { name: ja("kh.add"), exact: true }).first()).toBeVisible();
  await walk.look("known-hosts-scanned");

  for (const [path, screen] of [["/vault/passwords", "vault-passwords-add"], ["/vault/otp", "vault-otp-add"]] as const) {
    await goTo(page, path);
    await settle(page);
    await page.getByRole("button", { name: ja("secrets.add"), exact: true }).click();
    await walk.look(screen);
  }

  await goTo(page, "/snippets");
  await settle(page);
  await page.getByRole("main").getByRole("button", { name: /本番のバックアップ/ }).first().click();
  await walk.look("snippets-open");
  // A phone shows the list or the snippet; the list has the button to add one.
  const backToSnippets = page.getByRole("button", { name: ja("snippets.allSnippets"), exact: true });
  if (await backToSnippets.isVisible()) await backToSnippets.click();
  await page.getByRole("button", { name: ja("snippets.new"), exact: true }).first().click();
  await walk.look("snippets-add");

  await goTo(page, "/config");
  await settle(page);
  await page.getByRole("button", { name: "hosts.conf" }).first().click();
  await walk.look("config-file");

  await goTo(page, "/diagnostics");
  await settle(page);
  await page.getByRole("main").getByPlaceholder(ja("diag.hostAliasPlaceholder")).fill(longConnectionName);
  await page.getByRole("button", { name: ja("diag.explain"), exact: true }).click();
  await expect(page.getByRole("table", { name: ja("diag.sourcesCaption") })).toBeVisible();
  await walk.look("diagnostics-result");

  await goTo(page, "/");
  await settle(page);
  await page.getByRole("button", { name: ja("home.listView"), exact: true }).click();
  await walk.look("home-list");

  await goTo(page, "/settings/terminal");
  await settle(page);
  await page.getByLabel(ja("settings.presetToEdit")).first().selectOption({ label: longGroupName });
  await walk.look("settings-terminal-long-preset");
  for (const [tab, screen] of [["terminal.settingsGroupConnection", "settings-terminal-connection"], ["terminal.settingsGroupOperation", "settings-terminal-operation"]] as const) {
    const named = page.getByRole("tab", { name: ja(tab), exact: true });
    if (await named.isVisible()) {
      await named.click();
      await walk.look(screen);
    }
  }

  await goTo(page, "/settings/keybar");
  await settle(page);
  await page.getByRole("button", { name: ja("keyBar.createCustom") }).click();
  await walk.look("settings-keybar-custom-key");
  await page.keyboard.press("Escape");

  await goTo(page, "/settings/machines");
  await settle(page);
  await page.getByRole("button", { name: ja("settings.machineActions").replace("{name}", longConnectionName) }).click();
  await page.getByRole("menuitem", { name: ja("settings.machineRename") }).click();
  await walk.look("settings-machines-rename");
  await page.keyboard.press("Escape");

  await goTo(page, "/menu");
  await settle(page);
  await page.getByRole("button", { name: ja("menu.signOut") }).click();
  await walk.look("menu-sign-out");
  await page.keyboard.press("Escape");
}

// lookWithAnUpdateOut is last: the notice stays in the navigation of every page after it.
async function lookWithAnUpdateOut(page: Page, walk: Walk): Promise<void> {
  await answerUpdateAvailable(page);
  await page.reload();
  await expect(page.getByRole("main")).toBeVisible();
  await goTo(page, "/about");
  await walk.look("about-update-available");
  await goTo(page, "/");
  await walk.look("home-update-available");
}

async function setTheme(page: Page, theme: "dark" | "light"): Promise<void> {
  const failure = await page.evaluate(async (chosenTheme) => {
    const headers = { "Content-Type": "application/json", "X-SSHC-CSRF": window.sessionStorage.getItem("sshc.session.csrf") ?? "" };
    const settings = await (await fetch("/api/v1/settings", { credentials: "same-origin", headers })).json() as {
      presetInUse: string;
      presets: { id: string; appearance?: Record<string, unknown> }[];
    };
    const base = settings.presets.find((preset) => preset.id === settings.presetInUse);
    if (base === undefined) return "no preset is in use";
    const preset = { ...base, appearance: { ...base.appearance, theme: chosenTheme } };
    const saved = await fetch(`/api/v1/settings/presets/${base.id}`, { method: "PUT", credentials: "same-origin", headers, body: JSON.stringify({ base, preset }) });
    return saved.ok ? "" : `saving the theme answered ${saved.status}`;
  }, theme);
  expect(failure).toBe("");
  await page.reload();
  await expect(page.getByRole("main")).toBeVisible();
}

async function signIn(page: Page, installation: { url: string }, size: WindowSize, theme: "dark" | "light"): Promise<void> {
  watchRequests(page);
  await answerAbout(page);
  await openApplication(page, installation);
  await changeDisplayLanguage(page, "ja");
  await setTheme(page, theme);
  await page.setViewportSize(size);
}

for (const size of windowSizes) {
  for (const theme of themes) {
    test(`every page holds together over the worst data at ${size.width}x${size.height} (${theme})`, async ({ page, installation }) => {
      test.setTimeout(keptPicturesDirectory === undefined ? 360_000 : 900_000);
      await writeWorstCaseHome(installation);
      await answerWorstCaseHistory(page);
      await answerWorstCaseVPN(page);
      await answerConfiguredSync(page);
      await answerManyMachines(page);
      await answerWorstCaseScan(page);
      await answerInterruptedChange(page);
      await signIn(page, installation, size, theme);
      await addWorstCaseRecords(page);
      const walk = new Walk(page, `worst-${theme}`, size);
      await lookAtEveryPage(page, walk);
      await lookAtConnectionEditor(page, walk);
      await lookAtConnectionForms(page, walk, size);
      await lookAtGroups(page, walk, size);
      await lookAtPageForms(page, walk);
      await lookWithAnUpdateOut(page, walk);
      await walk.finish();
    });

    test(`every page holds together with nothing in it at ${size.width}x${size.height} (${theme})`, async ({ page, installation }) => {
      test.setTimeout(keptPicturesDirectory === undefined ? 180_000 : 400_000);
      await writeEmptyHome(installation);
      await answerManyMachines(page);
      await signIn(page, installation, size, theme);
      const walk = new Walk(page, `empty-${theme}`, size);
      await lookAtEveryPage(page, walk);
      await walk.finish();
    });
  }
}

test.describe("before the application opens", () => {
  test.use({ locale: "ja-JP" });

  for (const size of windowSizes) {
    for (const theme of themes) {
      test(`the vault screens and the failed start hold together at ${size.width}x${size.height} (${theme})`, async ({ page, installation }) => {
        watchRequests(page);
        await answerAbout(page);
        await page.emulateMedia({ colorScheme: theme });
        await page.setViewportSize(size);
        const walk = new Walk(page, `entry-${theme}`, size);

        await page.goto(installation.url);
        const passwords = page.locator('input[type="password"]');
        await expect(passwords.first()).toBeVisible();
        await page.getByLabel(ja("shell.themeMenu")).selectOption(theme);
        await walk.look("entry-create-vault");

        for (const field of await passwords.all()) await field.fill(masterPassword);
        await page.locator('button[type="submit"]').click();
        await expect(page.getByRole("main")).toBeVisible();
        await expect(passwords).toHaveCount(0);
        await page.evaluate(async () => {
          await fetch("/api/v1/passwords/lock", {
            method: "POST",
            credentials: "same-origin",
            headers: { "X-SSHC-CSRF": window.sessionStorage.getItem("sshc.session.csrf") ?? "" },
          });
        });
        await page.reload();
        await expect(passwords.first()).toBeVisible();
        await walk.look("entry-unlock");

        await page.route(/\/api\/v1\/session\//, (route) => route.abort());
        await page.reload();
        await expect(passwords).toHaveCount(0);
        await walk.look("entry-failed");
        await walk.finish();
      });
    }
  }
});

for (const size of windowSizes.filter(({ width }) => width < narrowestPCLayoutWidth)) {
  for (const theme of themes) {
    test(`every page holds together in the Android app at ${size.width}x${size.height} (${theme})`, async ({ page, installation }) => {
      test.setTimeout(keptPicturesDirectory === undefined ? 180_000 : 400_000);
      await writeEmptyHome(installation);
      watchRequests(page);
      await answerVPNWithoutDocker(page);
      await answerManyMachines(page);
      await behaveAsAndroidApp(page);
      await openApplication(page, installation);
      await changeDisplayLanguage(page, "ja");
      await setTheme(page, theme);
      await page.setViewportSize(size);
      const walk = new Walk(page, `android-${theme}`, size);
      await lookAtEveryPage(page, walk);
      await walk.finish();
    });
  }
}

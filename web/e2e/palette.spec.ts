import type { Page } from "@playwright/test";
import { chooseTheme, expect, openApplication, openLocalShell, openSection, test } from "./support/environment";
import { drawnRowFont, drawnRows, terminalKeyboard } from "./support/terminal";

function palette(page: Page) {
  return page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" });
}

// choose finds a command by what it is called and runs it, as a person does: Ctrl+K, a few words and Enter.
async function choose(page: Page, words: string, name: RegExp) {
  await page.keyboard.press("Control+k");
  await palette(page).getByRole("searchbox").fill(words);
  await expect(palette(page).getByRole("option").first()).toHaveAccessibleName(name);
  await page.keyboard.press("Enter");
  await expect(palette(page)).toBeHidden();
}

test("searches hosts, config files and settings from the keyboard", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", "Host r540\n  HostName 192.0.2.54\n  User aida\n");
  await openApplication(page, installation);
  // snippets.json is sealed with the Vault key, so the snippet is saved the way a person saves one.
  await openSection(page, "Snippets");
  // With no snippet yet, the empty list offers the same button as the page header; use the header's.
  await page.getByRole("main").locator("header").getByRole("button", { name: "Add snippet", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Update packages");
  await page.getByLabel("Command", { exact: true }).fill("sudo apt update");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByText("Snippet saved.", { exact: true })).toBeVisible();

  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" });
  await expect(palette).toBeVisible();

  const search = palette.getByRole("searchbox");
  await search.fill("r540");
  await expect(palette.getByRole("option", { name: /Connect to r540/ })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/command-palette-host-settings.png`, fullPage: true });
  }
  await palette.getByRole("button", { name: "Open connection settings for r540" }).click();
  await expect(page).toHaveURL(/\/connections\/servers\?path=conf\.d%2F20-lab\.conf&host=r540&panel=basic$/);
  await expect(page.getByRole("heading", { name: "r540", exact: true })).toBeVisible();
  await expect(palette).toBeHidden();

  await page.keyboard.press("Control+k");
  await search.fill("config");
  await expect(palette.getByRole("option", { name: /conf\.d\/20-lab\.conf/ })).toBeVisible();

  await search.fill("apt update");
  await palette.getByRole("option", { name: /Update packages/ }).click();
  await expect(page).toHaveURL(/\/snippets\?snippet=[0-9a-f]{32}$/);
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Update packages");

  await page.keyboard.press("Control+k");
  await search.fill("sync");
  await palette.getByRole("option", { name: /Sync/ }).click();
  await expect(page).toHaveURL(/\/sync$/);
  await expect(palette).toBeHidden();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.keyboard.press("Control+k");
    await expect(page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" })).toBeVisible();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.0-command-palette-desktop.png`, fullPage: true });
  }
});

test("opens the palette from the desktop toolbar", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Search everything" }).click();
  await expect(page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" })).toBeVisible();
});

test("runs the actions of a terminal by name, each with its key beside it, and they act on the terminal shown", async ({ page, installation, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openLocalShell(page);
  const rows = drawnRows(page);
  await expect(rows).toContainText(/[$#%>]/);
  const fontSize = async () => Number.parseFloat((await drawnRowFont(page)).fontSize);
  const before = await fontSize();
  await terminalKeyboard(page).focus();

  await page.keyboard.press("Control+k");
  await palette(page).getByRole("searchbox").fill("larger");
  const larger = palette(page).getByRole("option", { name: /Larger text/ });
  await expect(larger.locator("kbd")).toHaveText("Ctrl+=");
  await larger.click();
  await expect.poll(fontSize).toBe(before + 1);
  await choose(page, "reset text", /Reset text size/);
  await expect.poll(fontSize).toBe(before);

  // The search bar takes the keyboard, from the palette that held it.
  await choose(page, "terminal search", /Terminal search/);
  await expect(page.getByRole("textbox", { name: "Search terminal output" })).toBeFocused();
  await page.keyboard.press("Escape");

  await page.evaluate(() => navigator.clipboard.writeText("echo PASTED:$((6 * 7))"));
  await terminalKeyboard(page).focus();
  await choose(page, "paste", /^Action\s*Paste/);
  // The clipboard is read asynchronously, so Enter waits for the text: pressed first, it runs an empty line.
  await expect(rows).toContainText("echo PASTED:");
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Enter");
  await expect(rows).toContainText("PASTED:42");
});

test("shows the terminal first for an action of the terminal chosen on another page", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  await expect(drawnRows(page)).toContainText(/[$#%>]/);
  await openSection(page, "Home");

  await choose(page, "terminal search", /Terminal search/);
  await expect(page).toHaveURL(/\/terminal$/);
  await expect(page.getByRole("textbox", { name: "Search terminal output" })).toBeFocused();
});

test("splits, opens and closes sessions by name, and offers what needs a session only with one open", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.keyboard.press("Control+k");
  const options = palette(page).getByRole("option");
  await palette(page).getByRole("searchbox").fill("split");
  await expect(options).toHaveCount(0);
  await palette(page).getByRole("searchbox").fill("copy selection");
  await expect(options).toHaveCount(0);
  await palette(page).getByRole("searchbox").fill("new session");
  await options.first().click();
  await page.getByRole("dialog", { name: "New session" }).getByRole("button", { name: /^Local shell, localhost/ }).click();
  await expect(drawnRows(page)).toContainText(/[$#%>]/);

  await choose(page, "split right", /Split right/);
  await page.getByRole("dialog", { name: "New session on the right" }).getByRole("button", { name: /^Local shell, localhost/ }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);

  await choose(page, "remove from split", /Remove from split view/);
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);

  await choose(page, "close session", /Close session/);
  await expect(page.getByRole("dialog", { name: /^Close / })).toBeVisible();
});

test("changes the theme and OSC 52 by name, and says which is in use", async ({ page, installation }) => {
  await openApplication(page, installation);
  await chooseTheme(page, "dark");

  await choose(page, "theme white blue", /Theme: White blue/);
  await expect(page.locator("html")).toHaveAttribute("data-theme", "white-blue");
  await page.keyboard.press("Control+k");
  await palette(page).getByRole("searchbox").fill("theme");
  await expect(palette(page).getByRole("option", { name: /Theme: White blue/ })).toContainText("In use");
  const dark = palette(page).getByRole("option", { name: /Theme: Dark( In use)?$/ });
  await expect(dark).toHaveCount(1);
  await expect(dark).not.toContainText("In use");
  await page.keyboard.press("Escape");

  await choose(page, "osc 52", /Turn OSC 52 on/);
  await page.keyboard.press("Control+k");
  await palette(page).getByRole("searchbox").fill("osc 52");
  await expect(palette(page).getByRole("option").first()).toHaveAccessibleName(/Turn OSC 52 off/);
  await page.keyboard.press("Escape");
  await expect.poll(async () => JSON.parse(await installation.read("sshc/metadata.json")).settings?.terminal?.osc52).toBe(true);
});

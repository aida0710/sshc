import type { Page } from "@playwright/test";
import { expect, openApplication, openSection, test, type Installation } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

// Nothing listens on port 1 of this machine, so a session opened there ends at once with the refusal: these tests are
// about what the search opens and what it writes, not about logging in. The engine's side of logging in is covered by
// the sshclient, application and httpserver tests.
const destination = "deploy@127.0.0.1:1";

async function openNewSessionSearch(page: Page) {
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "New session", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "New session" });
  return { dialog, search: dialog.getByRole("searchbox", { name: "Search connections" }) };
}

async function hasNoSidewaysScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
}

test("the new session search connects to user@host without writing the config files", async ({ page, installation }) => {
  await openApplication(page, installation);
  const before = await installation.read("config");
  const { dialog, search } = await openNewSessionSearch(page);
  await search.fill(destination);

  await dialog.getByRole("button", { name: /^Connect/ }).click();
  await expect(dialog).toBeHidden();

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByText(destination).first()).toBeVisible();
  expect(await installation.read("config")).toBe(before);
});

test("the search connects with Enter, and leaves a plain search to the list", async ({ page, installation }) => {
  await openApplication(page, installation);
  const { dialog, search } = await openNewSessionSearch(page);
  await search.fill("nas");
  await page.keyboard.press("Enter");
  await expect(dialog).toBeVisible();

  await search.fill(destination);
  await page.keyboard.press("Enter");
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("navigation", { name: "Primary" }).getByText(destination).first()).toBeVisible();
});

test("Ctrl+K saves user@host as a Host block, connects, and lists it from then on", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" });
  await palette.getByRole("searchbox").fill(destination);

  await expect(palette.getByRole("option").first()).toContainText("Not saved to the config files");
  const save = palette.getByRole("option", { name: /Save as a connection and connect/ });
  await expect(save).toContainText("Connection name: 127-0-0-1");
  await save.click();
  await expect(palette).toBeHidden();

  await expect.poll(() => installation.read("config")).toContain("Host 127-0-0-1\n\tHostName 127.0.0.1\n\tUser deploy\n\tPort 1\n");
  await expect(page.getByRole("navigation", { name: "Primary" }).getByText("127-0-0-1").first()).toBeVisible();
  // No password or key is written for it: the terminal asks when connecting.
  expect(await installation.read("config")).not.toContain("IdentityFile ~/.ssh/127");

  const { dialog, search } = await openNewSessionSearch(page);
  await search.fill("127-0-0-1");
  await expect(dialog.getByRole("button", { name: /^127-0-0-1/ })).toBeVisible();
});

async function openNewConnection(page: Page, installation: Installation) {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("button", { name: "Add connection", exact: true }).first().click();
  return page.getByRole("form", { name: "New connection" });
}

test("a new connection with an empty name takes it from the host name", async ({ page, installation }) => {
  const form = await openNewConnection(page, installation);
  await form.getByLabel("Host name or IP address").fill("203.0.113.77");
  await expect(form.getByLabel("Connection name")).toHaveAttribute("placeholder", "203-0-113-77");

  await form.getByLabel("Authentication method").selectOption({ label: "Enter when connecting" });
  await form.getByRole("button", { name: "Create connection" }).click();

  await expect(page.getByRole("heading", { name: "203-0-113-77", exact: true })).toBeVisible();
  const config = await installation.read("config");
  expect(config).toContain("Host 203-0-113-77\n\tHostName 203.0.113.77\n\tPort 22\n");
});

test("a new connection chooses the connection it goes through, on the screen that creates it", async ({ page, installation }) => {
  const form = await openNewConnection(page, installation);
  const jump = form.getByLabel("Connect through");
  await expect(jump).toHaveValue("");
  await expect(jump.getByRole("option", { selected: true })).toHaveText("None (connect directly)");
  await form.getByLabel("Connection name").fill("inner");
  await form.getByLabel("Host name or IP address").fill("10.0.0.9");
  await jump.selectOption("bastion");
  await form.getByLabel("Authentication method").selectOption({ label: "Enter when connecting" });
  await form.getByRole("button", { name: "Create connection" }).click();

  await expect(page.getByRole("heading", { name: "inner", exact: true })).toBeVisible();
  expect(await installation.read("config")).toContain("Host inner\n\tHostName 10.0.0.9\n\tPort 22\n\tProxyJump bastion\n");
});

test("a connection made on the Connections page is in the new session search at once", async ({ page, installation }) => {
  const form = await openNewConnection(page, installation);
  await form.getByLabel("Host name or IP address").fill("web.example.com");
  await form.getByLabel("Authentication method").selectOption({ label: "Enter when connecting" });
  await form.getByRole("button", { name: "Create connection" }).click();
  await expect(page.getByRole("heading", { name: "web", exact: true })).toBeVisible();

  const { dialog, search } = await openNewSessionSearch(page);
  await search.fill("web");
  await expect(dialog.getByRole("button", { name: /^web/ })).toBeVisible();
  // Saving the same host again takes the next name.
  await search.fill("deploy@web.example.com");
  await expect(dialog.getByRole("button", { name: /Connection name: web-2/ })).toBeVisible();
});

test("the fields of a new connection fit a phone", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.goto(new URL("/connections", page.url()).href);
  await page.locator("[data-app-header]").getByRole("button", { name: "Add connection", exact: true }).click();
  const form = page.getByRole("form", { name: "New connection" });
  const formBox = await form.boundingBox();
  if (formBox === null) throw new Error("the form is not drawn");
  for (const label of ["Connection name", "Host name or IP address", "Connect through", "Authentication method"]) {
    const box = await form.getByLabel(label).boundingBox();
    if (box === null) throw new Error(`${label} is not drawn`);
    expect(box.x + box.width).toBeLessThanOrEqual(formBox.x + formBox.width);
  }
  await hasNoSidewaysScroll(page);
});

test("the search for a new session fits a phone", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().click();
  const { dialog, search } = await openNewSessionSearch(page);
  await search.fill("a-rather-long-user-name@a-rather-long-host-name.example.com:2222");
  const save = dialog.getByRole("button", { name: /^Save as a connection and connect/ });
  await expect(save).toBeVisible();
  const box = await save.boundingBox();
  if (box === null) throw new Error("the row is not drawn");
  expect(box.x + box.width).toBeLessThanOrEqual(390);
  await hasNoSidewaysScroll(page);
});

// The e2e suite has no SSH server to type a password to, so the engine's side is played by the session list and the
// answer it would give.
const asking = {
  id: "password-fixture",
  kind: "ssh", workspaceId: "default",
  alias: "web-01",
  title: "web-01",
  startedAt: "2026-11-04T00:00:00Z",
  state: "connected",
  problem: "",
  passwordSaveQuestion: true,
  forwards: [],
};
const { passwordSaveQuestion: _question, ...answered } = asking;

// playEngine answers for one session that logs in once the terminal is on the screen, as a person's does: the question
// arrives with the next list after ask().
async function playEngine(page: Page, refusal?: { status: number; code: string }) {
  const answers: unknown[] = [];
  let session: Record<string, unknown> = answered;
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/password-save")) {
      answers.push(route.request().postDataJSON());
      if (refusal !== undefined) {
        await route.fulfill({ status: refusal.status, contentType: "application/problem+json", body: JSON.stringify({ code: refusal.code, message: refusal.code }) });
        return;
      }
      session = answered;
    }
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "password-save" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => { socket.send(Buffer.from("deploy@web-01:~$ ")); });
  return { answers, ask: () => { session = asking; } };
}

const question = "Save this password to the Vault for this connection?";

test("asks once whether to save a typed password, and the answer ends the question", async ({ page, installation }) => {
  const { answers, ask } = await playEngine(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toBeVisible();
  ask();
  const card = screen.getByRole("region", { name: question });
  await expect(card).toBeFocused();
  // An Enter meant for the shell does not store a password.
  await page.keyboard.press("Enter");
  expect(answers).toEqual([]);

  await card.getByRole("button", { name: "Save", exact: true }).click();
  await expect.poll(() => answers).toEqual([{ save: true }]);
  await expect(card).toBeHidden();
  await page.waitForTimeout(100);
  await expect(card).toBeHidden();
});

test("Escape answers no and gives the keyboard back to the terminal", async ({ page, installation }) => {
  const { answers, ask } = await playEngine(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  ask();
  const card = page.getByRole("region", { name: question });
  await expect(card).toBeFocused();
  await page.keyboard.press("Escape");
  await expect.poll(() => answers).toEqual([{ save: false }]);
  await expect(card).toBeHidden();
  await expect(terminalKeyboard(page).first()).toBeFocused();
});

test("a refused answer keeps the question open and says so", async ({ page, installation }) => {
  const { ask } = await playEngine(page, { status: 409, code: "connection_changed" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  ask();
  const card = page.getByRole("region", { name: question });
  await card.getByRole("button", { name: "Save", exact: true }).click();
  await expect(card).toContainText("The password could not be saved.");
});

test("a phone keeps the question behind the notices button until asked", async ({ page, installation }) => {
  const { answers, ask } = await playEngine(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().click();
  await page.getByRole("button", { name: /^web-01/ }).first().click();
  await expect(page.getByRole("button", { name: "Notices", exact: true })).toHaveCount(0);

  ask();
  const notices = page.getByRole("button", { name: "Notices", exact: true });
  await expect(notices).toBeVisible();
  const card = page.getByRole("region", { name: question });
  await expect(card).toBeHidden();
  await notices.click();
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "Don't save", exact: true }).click();
  await expect.poll(() => answers).toEqual([{ save: false }]);
  await expect(card).toBeHidden();
  await expect(page.getByRole("button", { name: "Notices", exact: true })).toHaveCount(0);
});

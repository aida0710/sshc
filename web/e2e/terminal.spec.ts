import {
  expect,
  type Installation,
  openApplication,
  openSection,
  openSettingsPage,
  shellSays,
  test,
} from "./support/environment";
import type { Locator, Page, Response } from "@playwright/test";
import {
  drawnRowCount,
  drawnRowFont,
  drawnRows,
  drawnSpan,
  markTerminal,
  selectionMarks,
  surfaceBackgroundImage,
  surfaceToken,
  terminalCanvasCount,
  terminalFitRects,
  terminalKeyboard,
  terminalScreen,
  viewportBackground,
} from "./support/terminal";

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

async function typeIntoConsole(page: import("@playwright/test").Page, line: string) {
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
  return screen;
}

async function openConsolePanel(page: import("@playwright/test").Page) {
  const nav = page.getByRole("navigation", { name: "Primary" });
  await expect(nav.getByRole("button", { name: "Local shell" })).toBeVisible();
  return nav;
}

async function reopenFirstConsole(panel: Locator) {
  await panel
    .getByRole("list", { name: "Open consoles" })
    .getByRole("listitem")
    .first()
    .getByRole("button")
    .first()
    .click();
}

async function seedTerminalSettings(installation: Installation) {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({
      schemaVersion: 3,
      embeddedTerminal: { maxSessions: 2, scrollbackBytes: 16384 },
    }),
  );
}

async function openLoadedTerminalSettings(page: Page): Promise<Locator> {
  await openSettingsPage(page, "Terminal");
  const region = page.getByRole("region", { name: "Terminal" });
  // The sentinel proves that the Settings effect has applied its metadata GET.
  // Without this gate, a slow initial GET can overwrite a value filled by the test.
  await expect(region.getByLabel("Consoles open at once")).toHaveValue("2");
  return region;
}

async function saveTerminalSettings(page: Page, region: Locator): Promise<Response> {
  const save = region.getByRole("button", { name: "Save" });
  const responsePromise = page.waitForResponse((response) => {
    const request = response.request();
    return new URL(response.url()).pathname === "/api/v1/metadata/terminal" && request.method() === "PUT";
  });
  await save.click();
  const response = await responsePromise;
  expect(response.ok()).toBe(true);
  await expect(save).toBeEnabled();
  return response;
}

test("opens a local shell, runs a command and shows its output", async ({ page, installation }) => {
  const violations = watchForPolicyViolations(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const emptyState = page.getByRole("heading", { name: "No console is open" }).locator("..");
  await expect(emptyState.locator("[data-sshc-brand-mark]")).toBeVisible();
  await expect(emptyState).not.toContainText(">_");
  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();

  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();

  await typeIntoConsole(page, "echo embedded-terminal-canary");

  await expect(screen).toContainText("embedded-terminal-canary", { timeout: 20_000 });
  expect(violations).toEqual([]);
});

test("uses the transparent-safe renderer for a local shell with a background image", async ({ page, installation }) => {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({
      schemaVersion: 4,
      embeddedTerminal: { appearance: { background: "terminal-wall.gif" } },
    }),
  );
  await page.route("**/api/v1/terminal/backgrounds/terminal-wall.gif", async (route) => {
    await route.fulfill({
      body: Buffer.from("R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==", "base64"),
      contentType: "image/gif",
    });
  });
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  await expect(screen.locator("[data-term-background='terminal-wall.gif']")).toBeVisible();
  await expect.poll(() => terminalCanvasCount(page)).toBe(0);

  await terminalKeyboard(page).focus();
  await page.keyboard.type("printf stale");
  await page.keyboard.press("Backspace");
  await page.keyboard.press("Backspace");
  await page.keyboard.press("Backspace");
  await page.keyboard.press("Backspace");
  await page.keyboard.press("Backspace");
  await page.keyboard.type("fixed");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("fixed", { timeout: 20_000 });
});

test("keeps the session and replays its scrollback after a reload", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const screen = await typeIntoConsole(page, "echo survives-a-reload");
  await expect(screen).toContainText("survives-a-reload", { timeout: 20_000 });

  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();
  const reopened = await openConsolePanel(page);
  const row = reopened.getByRole("list", { name: "Open consoles" }).getByRole("listitem").first();
  await expect(row).toBeVisible();
  await row.getByRole("button").first().click();

  await expect(page.getByRole("region", { name: /^Console for / }))
    .toContainText("survives-a-reload", { timeout: 20_000 });
});

test("refuses to open more consoles than the configured limit", async ({ page, installation }) => {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({ schemaVersion: 3, embeddedTerminal: { maxSessions: 2, scrollbackBytes: 16384 } }),
  );
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const openShell = panel.getByRole("button", { name: "Local shell" });

  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await openShell.click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await expect(rows).toHaveCount(1);
  await openShell.click();
  await expect(rows).toHaveCount(2);

  await expect(openShell).toBeDisabled();
  await expect(panel).toContainText("limit of 2 open consoles");
});

test("shows an open console again after a reload instead of claiming there are none", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();

  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await expect(page.getByText("No console is open")).toBeHidden();
});

test("applies the session limit set from the settings screen", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const region = await openLoadedTerminalSettings(page);
  await region.getByLabel("Consoles open at once").fill("1");
  const saved = await saveTerminalSettings(page, region);
  expect(saved.request().postDataJSON()).toEqual(expect.objectContaining({ maxSessions: 1 }));
  expect(JSON.parse(await installation.read("sshc/metadata.json"))).toEqual(
    expect.objectContaining({ embeddedTerminal: expect.objectContaining({ maxSessions: 1 }) }),
  );

  const panel = await openConsolePanel(page);
  const openShell = panel.getByRole("button", { name: "Local shell" });
  const created = page.waitForResponse((response) => {
    const request = response.request();
    return new URL(response.url()).pathname === "/api/v1/terminal/sessions" && request.method() === "POST";
  });
  const refreshed = page.waitForResponse((response) => {
    const request = response.request();
    return new URL(response.url()).pathname === "/api/v1/terminal/sessions" && request.method() === "GET";
  });
  await openShell.click();
  expect((await created).status()).toBe(201);
  const listed = await refreshed;
  expect((await listed.json()).maxSessions).toBe(1);
  const consoles = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await expect(consoles).toHaveCount(1);

  await expect(openShell).toBeDisabled();
  await expect(panel).toContainText("limit of 1 open consoles");
});

test("starts local shells where the setting says", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await installation.write("../workspace/marker", "");
  await openApplication(page, installation);

  const region = await openLoadedTerminalSettings(page);
  await region.getByLabel("Starting directory").fill("~/workspace");
  await saveTerminalSettings(page, region);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "pwd");
  await expect(screen).toContainText(
    process.platform === "win32" ? "\\workspace" : "/workspace",
    { timeout: 20_000 },
  );
});

test("copies what was selected in the console as soon as selection finishes", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "echo selectable-canary");
  await expect(screen).toContainText("selectable-canary", { timeout: 20_000 });

  const rows = drawnRows(page);
  const box = await rows.boundingBox();
  expect(box).not.toBeNull();
  if (box === null) return;
  await page.mouse.move(box.x + 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 2, box.y + box.height - 2, { steps: 12 });
  await page.mouse.up();
  await expect(selectionMarks(page).first()).toBeAttached();
  await expect
    .poll(async () => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("selectable-canary");
});

test("pastes the clipboard into the console with right click", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.evaluate(() => navigator.clipboard.writeText("echo right-click-paste-canary"));

  await terminalScreen(page).click({ button: "right", position: { x: 20, y: 20 } });
  await page.keyboard.press("Enter");

  await expect(screen).toContainText("right-click-paste-canary", { timeout: 20_000 });
});

test("pastes a desktop keyboard shortcut into the console only once", async ({ page, context, installation }) => {
  const inputFrames: string[] = [];
  page.on("websocket", (socket) => socket.on("framesent", ({ payload }) => {
    inputFrames.push(typeof payload === "string" ? payload : payload.toString("utf8"));
  }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await typeIntoConsole(page, 'rm -f "$HOME/keyboard-paste-data"');
  await page.evaluate(() => navigator.clipboard.writeText('printf x >> "$HOME/keyboard-paste-data"; '));

  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Shift+V");
  await page.keyboard.press("Enter");
  const pastedFrames = inputFrames.filter((frame) => frame.includes("keyboard-paste-data"));
  expect(pastedFrames, JSON.stringify(inputFrames)).toHaveLength(1);
  expect(
    pastedFrames[0]!.split("keyboard-paste-data").length - 1,
    JSON.stringify(pastedFrames[0]),
  ).toBe(1);
  await typeIntoConsole(page, 'echo keyboard-paste-count=$(wc -c < "$HOME/keyboard-paste-data")');

  await expect(screen).toContainText("keyboard-paste-count=1", { timeout: 20_000 });
});

test("keeps terminal drawing stable while scrollback search overlays it", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const console = page.getByRole("region", { name: /^Console for / });
  await expect(console).toContainText(/[$#%>]/, { timeout: 20_000 });
  await typeIntoConsole(page, "echo search-overlay-canary");
  await expect(console).toContainText("search-overlay-canary", { timeout: 20_000 });
  const before = await terminalFitRects(page);

  await console.getByRole("button", { name: "Find" }).click();
  const searchInput = console.getByRole("textbox", { name: "Search terminal output" });
  await expect(searchInput).toBeVisible();
  await console.getByRole("button", { name: "Use regular expression" }).click();
  await searchInput.fill("search-overlay-(canary|missing)");
  await expect(searchInput.locator("..").getByRole("status")).toHaveText(/\d+\/\d+/);
  const opened = await terminalFitRects(page);
  expect(Math.abs(opened.root.y - before.root.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(opened.root.height - before.root.height)).toBeLessThanOrEqual(1);
  expect(opened.root.y + opened.root.height).toBeLessThanOrEqual(opened.host.y + opened.host.height + 1);

  await console.getByRole("button", { name: "Close search" }).click();
  await expect(console.getByRole("textbox", { name: "Search terminal output" })).toHaveCount(0);
  await expect.poll(async () => Math.abs((await terminalFitRects(page)).root.height - before.root.height)).toBeLessThanOrEqual(1);
});

test("can turn automatic selection copy off for an already open console", async ({
  page,
  context,
  installation,
}) => {
  await seedTerminalSettings(installation);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "echo copy-setting-canary");
  await expect(screen).toContainText("copy-setting-canary", { timeout: 20_000 });

  const settings = await openLoadedTerminalSettings(page);
  await settings.getByRole("checkbox", { name: "Copy selected text automatically" }).uncheck();
  await saveTerminalSettings(page, settings);

  await reopenFirstConsole(panel);
  await page.evaluate(() => navigator.clipboard.writeText("clipboard-sentinel"));
  const rows = drawnRows(page);
  const box = await rows.boundingBox();
  expect(box).not.toBeNull();
  if (box === null) return;
  await page.mouse.move(box.x + 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 2, box.y + box.height - 2, { steps: 12 });
  await page.mouse.up();
  await expect(selectionMarks(page).first()).toBeAttached();

  await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe("clipboard-sentinel");
});

test("fits the terminal inside the space it was given", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const measured = await drawnRows(page).evaluate((node) => {
    const frame = (node as HTMLElement).closest("main");
    return {
      rows: Math.round((node as HTMLElement).getBoundingClientRect().height),
      frame: Math.round(frame?.getBoundingClientRect().height ?? 0),
    };
  });
  expect(measured.frame).toBeGreaterThan(0);
  expect(measured.rows).toBeLessThanOrEqual(measured.frame);
});

test("keeps the top of the navigation still while the console list scrolls", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const anchor = panel.getByRole("link", { name: "Connections", exact: true });
  const before = await anchor.boundingBox();

  await page.setViewportSize({ width: 1280, height: 400 });
  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  for (let opened = 1; opened <= 6; opened += 1) {
    await panel.getByRole("button", { name: "Local shell" }).click();
    await expect(rows).toHaveCount(opened);
  }

  const scroller = page.locator("nav[aria-label='Primary'] div.overflow-y-auto");
  await expect(async () => {
    expect(await scroller.evaluate((node) => node.scrollHeight - node.clientHeight)).toBeGreaterThan(0);
  }).toPass();
  await scroller.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });

  const after = await anchor.boundingBox();
  expect(after?.y).toBe(before?.y);
});

test("closes every open connection from the settings screen", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(rows).toHaveCount(1);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(rows).toHaveCount(2);

  await openSettingsPage(page, "Open connections");
  const region = page.getByRole("region", { name: "Open connections" });
  await expect(region.getByText("2 open")).toBeVisible();
  await region.getByRole("button", { name: "Close every connection" }).click();
  await page.getByRole("button", { name: "Close them all" }).click();

  await expect(region.getByText("0 open")).toBeVisible();
  await openConsolePanel(page);
  await expect(rows).toHaveCount(0);
});

test("force closes a live local shell with one confirmation", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(rows).toHaveCount(1);

  await panel.getByRole("button", { name: /^Close / }).click();
  const closeResponse = page.waitForResponse((response) => {
    const request = response.request();
    return request.method() === "DELETE" && /\/api\/v1\/terminal\/sessions\/[^/]+$/.test(new URL(response.url()).pathname);
  });
  await page.getByRole("dialog").getByRole("button", { name: "Close", exact: true }).click();
  expect((await closeResponse).ok()).toBe(true);

  await expect(rows).toHaveCount(0);
});

test("previews and closes a live connection immediately while Shift is held", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  const rows = panel.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(rows).toHaveCount(1);

  const row = rows.first();
  const card = row.locator(":scope > div").first();
  await page.keyboard.down("Shift");
  await expect(card).toHaveClass(/bg-danger\/10/);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-shift-close.png` });
  }

  const closeResponse = page.waitForResponse((response) => {
    const request = response.request();
    return request.method() === "DELETE" && /\/api\/v1\/terminal\/sessions\/[^/]+$/.test(new URL(response.url()).pathname);
  });
  await row.getByRole("button", { name: /^Close / }).click();
  expect((await closeResponse).ok()).toBe(true);
  await page.keyboard.up("Shift");

  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(rows).toHaveCount(0);
});

test("moves to its own screen and leaves the connection detail alone", async ({ page, installation }) => {
  await installation.write("conf.d/20-detail.conf", ["Host detail-host", "\tHostName 127.0.0.1", ""].join("\n"));
  await openApplication(page, installation);

  await openSection(page, "Connections");
  const tree = page.getByRole("navigation", { name: "Connections" });
  await tree.getByRole("button", { name: "detail-host" }).click();
  const detail = page.getByRole("heading", { name: "detail-host" });
  await expect(detail).toBeVisible();

  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect(page).toHaveURL(/\/terminal$/);
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
  await expect(tree).toBeHidden();

  await page.goBack();
  await expect(page).not.toHaveURL(/\/terminal$/);
  await expect(detail).toBeVisible();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeHidden();
});

test("shows why a connection failed in the console itself", async ({ page, installation }) => {
  await installation.write(
    "conf.d/20-refused.conf",
    ["Host refused", "\tHostName 127.0.0.1", "\tPort 1", "\tConnectTimeout 2", ""].join("\n"),
  );
  await openApplication(page, installation);

  await openSection(page, "Connections");
  const nav = page.getByRole("navigation", { name: "Connections" });
  await nav.getByRole("button", { name: "refused" }).click();
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/sshc:.*(refused|connect)/i, { timeout: 20_000 });
});

test("tells the pseudo-terminal how big it is as soon as it attaches", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, shellSays.size);
  await expect(screen).toContainText(/\d+-\d+/, { timeout: 20_000 });

  const reported = (await screen.innerText()).match(/(\d+)-(\d+)/);
  expect(reported).not.toBeNull();
  const drawn = await drawnRowCount(page);
  expect(Number(reported?.[1])).toBe(drawn);
  expect(Number(reported?.[2])).toBeGreaterThan(80);
});

test("keeps the same terminal alive while another screen is shown", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, "echo stays-mounted");
  await expect(screen).toContainText("stays-mounted", { timeout: 20_000 });

  const sameTerminal = await markTerminal(page, "1");
  await typeIntoConsole(page, shellSays.lateEcho("late-42"));

  await openSettingsPage(page, "Engine");
  await expect(screen).toBeHidden();

  const reopened = await openConsolePanel(page);
  await reopened
    .getByRole("list", { name: "Open consoles" })
    .getByRole("listitem")
    .first()
    .getByRole("button")
    .first()
    .click();

  await expect(screen).toBeVisible();
  await expect(sameTerminal).toBeAttached();
  await expect(screen).toContainText("stays-mounted");
  await expect(screen).toContainText("late-42", { timeout: 20_000 });
});

test("does not hand npm's own environment to the shell", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, 'echo "prefix=[${npm_config_prefix}]"');

  await expect(screen).toContainText("prefix=[]", { timeout: 20_000 });
});

test("paints the console in the colour scheme that was chosen", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  const screen = await typeIntoConsole(page, shellSays.redWord("zzred"));
  await expect(screen).toContainText("zzred", { timeout: 20_000 });

  const surface = () => surfaceToken(page, "--ui-term-bg");
  const drawnRed = () =>
    drawnSpan(page, "zzred")
      .last()
      .evaluate((node) => getComputedStyle(node as HTMLElement).color);

  const beforeSurface = await surface();
  const beforeRed = await drawnRed();

  const settings = await openLoadedTerminalSettings(page);
  await settings.getByLabel("Colour scheme").selectOption("dracula");
  await saveTerminalSettings(page, settings);

  await reopenFirstConsole(panel);
  await expect.poll(surface).toBe("#282a36");
  await expect.poll(drawnRed).toBe("rgb(255, 85, 85)");
  expect(beforeSurface).not.toBe("#282a36");
  expect(beforeRed).not.toBe("rgb(255, 85, 85)");
});

test("loads the font it ships and hands it to the console", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const family = async () => (await drawnRowFont(page)).fontFamily;
  expect(await family()).not.toContain("JetBrains Mono");

  const settings = await openLoadedTerminalSettings(page);
  await settings.getByLabel("Font family").selectOption("jetbrains-mono");
  await saveTerminalSettings(page, settings);

  await reopenFirstConsole(panel);
  await expect.poll(family).toContain("JetBrains Mono");
  await expect
    .poll(async () => page.evaluate(() => document.fonts.check('13px "JetBrains Mono"')))
    .toBe(true);
});

test("wears the image that was brought in, and gets out of its way", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const panel = await openConsolePanel(page);
  await panel.getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();

  const encoded = await page.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = 4;
    canvas.height = 4;
    const context = canvas.getContext("2d")!;
    context.fillStyle = "rgb(0, 200, 0)";
    context.fillRect(0, 0, 4, 4);
    return canvas.toDataURL("image/png");
  });

  const settings = await openLoadedTerminalSettings(page);
  await settings.locator('input[type="file"]').setInputFiles({
    name: "Canary Wall.png",
    mimeType: "image/png",
    buffer: Buffer.from(encoded.split(",")[1] ?? "", "base64"),
  });
  const thumbnail = settings.getByRole("img", { name: "canary-wall.png" });
  await expect(thumbnail).toBeVisible();
  await expect.poll(async () => thumbnail.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
  await saveTerminalSettings(page, settings);

  await reopenFirstConsole(panel);
  const wiring = {
    image: await surfaceBackgroundImage(page),
    viewport: await viewportBackground(page),
  };
  expect(wiring.image).toContain("data:image/png");
  expect(wiring.image).toMatch(/^linear-gradient\(/);
  expect(wiring.viewport).toBe("rgba(0, 0, 0, 0)");
});

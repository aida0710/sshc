import { readFile } from "node:fs/promises";
import { expect, type Installation, openApplication, openSection, openSettingsPage, shellSays, test, openLocalShell } from "./support/environment";
import { metadataSchemaVersion, standardPresetMetadata } from "./support/settings";
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
import { watchForPolicyViolations } from "./support/policyViolations";

async function typeIntoTerminal(page: import("@playwright/test").Page, line: string) {
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
  return screen;
}

async function openSessionList(page: import("@playwright/test").Page) {
  const nav = page.getByRole("navigation", { name: "Primary" });
  await expect(nav.getByRole("button", { name: "New session" })).toBeVisible();
  return nav;
}

async function reopenFirstSession(panel: Locator) {
  await panel
    .getByRole("list", { name: "Open sessions" })
    .getByRole("listitem")
    .first()
    .getByRole("button")
    .first()
    .click();
}

async function seedTerminalSettings(installation: Installation) {
  await installation.write(
    "sshc/metadata.json",
    standardPresetMetadata({ terminal: { maxSessions: 2, scrollbackBytes: 16384 } }),
  );
}

// openLoadedTerminalSettings leaves Settings > Terminal on its Connection tab, where the sentinel is.
async function openLoadedTerminalSettings(page: Page): Promise<Locator> {
  await openSettingsPage(page, "Terminal");
  const region = page.getByRole("region", { name: "Terminal" });
  await openTerminalSettingsTab(region, "Connection");
  // The sentinel proves that the Settings effect has applied its metadata GET.
  // Without this gate, a slow initial GET can overwrite a value filled by the test.
  await expect(region.getByLabel("Sessions open at once")).toHaveValue("2");
  return region;
}

async function openTerminalSettingsTab(region: Locator, name: "Display" | "Connection" | "Behaviour"): Promise<void> {
  await region.getByRole("tab", { name, exact: true }).click();
}

// saveTerminalSettings waits for the change just made to be saved. The page has no Save button: a choice is
// saved once it rests, and a typed field when it is left, which Tab does.
async function saveTerminalSettings(page: Page, region: Locator): Promise<Response> {
  const responsePromise = page.waitForResponse((response) => {
    const request = response.request();
    return new URL(response.url()).pathname === "/api/v1/settings/presets/default" && request.method() === "PUT";
  });
  await region.page().keyboard.press("Tab");
  const response = await responsePromise;
  expect(response.ok()).toBe(true);
  return response;
}

test("opens a local shell, runs a command and shows its output", async ({ page, installation }) => {
  const violations = watchForPolicyViolations(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const emptyState = page.getByRole("heading", { name: "No session is open" }).locator("..");
  await expect(emptyState.locator("[data-sshc-brand-mark]")).toBeVisible();
  await expect(emptyState).not.toContainText(">_");
  await openSessionList(page);
  await openLocalShell(page);

  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toBeVisible();

  await typeIntoTerminal(page, "echo embedded-terminal-canary");

  await expect(screen).toContainText("embedded-terminal-canary", { timeout: 20_000 });
  if (process.platform !== "win32") {
    await typeIntoTerminal(page, 'printf "TERM=%s\\n" "$TERM"');
    await expect(screen).toContainText("TERM=xterm-256color", { timeout: 20_000 });
  }
  expect(violations).toEqual([]);
});

test("keeps image-backed terminal editing free of stale glyphs", async ({ page, installation }) => {
  await installation.write(
    "sshc/metadata.json",
    standardPresetMetadata({ terminal: { appearance: { background: "terminal-wall.gif" } } }),
  );
  await page.route("**/api/v1/terminal/backgrounds/terminal-wall.gif", async (route) => {
    await route.fulfill({
      body: Buffer.from("R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==", "base64"),
      contentType: "image/gif",
    });
  });
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
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

test("keeps WebGL disabled when the terminal setting turns it off", async ({ page, installation }) => {
  await installation.write(
    "sshc/metadata.json",
    standardPresetMetadata({ terminal: { webgl: false } }),
  );
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toBeVisible();
  await expect.poll(() => terminalCanvasCount(page)).toBe(0);

  await typeIntoTerminal(page, "printf webgl-disabled");
  await expect(screen).toContainText("webgl-disabled", { timeout: 20_000 });
});

test("keeps the session and replays its scrollback after a reload", async ({ page, installation }) => {
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();

  const screen = await typeIntoTerminal(page, "echo survives-a-reload");
  await expect(screen).toContainText("survives-a-reload", { timeout: 20_000 });

  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();
  const reopened = await openSessionList(page);
  const row = reopened.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first();
  await expect(row).toBeVisible();
  await row.getByRole("button").first().click();

  await expect(page.getByRole("region", { name: /^Terminal for / }))
    .toContainText("survives-a-reload", { timeout: 20_000 });
});

test("does not answer the queries in replayed output again after a reload", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf and read are POSIX shell syntax here");
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);

  // The program asks for the terminal's identity (DA1) and reads the answer,
  // as vim and Claude Code do when they start.
  const screen = await typeIntoTerminal(page, "printf '\\033[c'; IFS= read -rs -d c reply; echo answer-$((1+1))-read");
  await expect(screen).toContainText("answer-2-read", { timeout: 20_000 });

  await page.reload();
  const sentAfterReload: string[] = [];
  page.on("websocket", (socket) => {
    socket.on("framesent", ({ payload }) => {
      sentAfterReload.push(typeof payload === "string" ? payload : payload.toString("utf8"));
    });
  });
  const reopened = await openSessionList(page);
  await reopened.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first().getByRole("button").first().click();
  await expect(screen).toContainText("answer-2-read", { timeout: 20_000 });

  await typeIntoTerminal(page, "echo typed-after-$((20+6))");
  await expect(screen).toContainText("typed-after-26", { timeout: 20_000 });
  expect(sentAfterReload.join("")).not.toContain("\u001b[?");
});

test("says only the oldest output is gone, not that the link was offline, when a reloaded page shows a session", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "seq is POSIX here");
  await installation.write(
    "sshc/metadata.json",
    standardPresetMetadata({ terminal: { scrollbackBytes: 16384 } }),
  );
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  // More output than the 16 KiB the engine keeps for the session.
  const screen = await typeIntoTerminal(page, "seq -f 'line %06g' 4000; echo printed-$((40+2))");
  await expect(screen).toContainText("printed-42", { timeout: 20_000 });

  // A reloaded page builds the terminal anew from the engine's replay.
  await page.reload();
  const reopened = await openSessionList(page);
  await reopened.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first().getByRole("button").first().click();

  await expect(page.getByText("The session's oldest output is no longer in the engine replay buffer, so it is not shown here.")).toBeVisible();
  await expect(page.getByText(/while the terminal link was offline/)).toHaveCount(0);
});

test("keeps the whole scrollback of a session shown again, past what the engine replays", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "seq and printf are POSIX here");
  await openApplication(page, installation);
  const panel = await openSessionList(page);
  await openLocalShell(page);
  // About 360 KB: fewer lines than the browser keeps, more bytes than the engine's 256 KiB replay.
  const screen = await typeIntoTerminal(page, "for i in $(seq -w 1 3000); do printf 'line %s %0110d\\n' $i 0; done; echo printed-$((40+2))");
  await expect(screen).toContainText("printed-42", { timeout: 30_000 });
  await openLocalShell(page);
  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await expect(rows).toHaveCount(2);

  await rows.first().getByRole("button").first().click();
  await expect(screen).toContainText("printed-42", { timeout: 20_000 });

  // The scrollback search reads the whole buffer: the first line is still there.
  await screen.getByRole("button", { name: "Find" }).click();
  await screen.getByRole("textbox", { name: "Search terminal output" }).fill("line 0001 0");
  await expect(screen.getByRole("textbox", { name: "Search terminal output" }).locator("..").getByRole("status")).toHaveText("1/1");
  await expect(page.getByText(/no longer in the engine replay buffer/)).toHaveCount(0);
});

test("keeps as many terminals hidden as the setting says", async ({ page, installation }) => {
  await installation.write(
    "sshc/metadata.json",
    standardPresetMetadata({ terminal: { retainedTerminals: 1 } }),
  );
  await openApplication(page, installation);
  const panel = await openSessionList(page);
  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  for (let opened = 1; opened <= 3; opened += 1) {
    await openLocalShell(page);
    await expect(rows).toHaveCount(opened);
  }

  // The two shown before the last one would both stay with the default of 3; the setting keeps one.
  await expect(page.locator("[data-split-view-kept]")).toHaveCount(1);
});

test("keeps the link and stops a program flooding the terminal at once on Ctrl+C", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "yes is POSIX here");
  await openApplication(page, installation);
  await openSessionList(page);
  let sockets = 0;
  page.on("websocket", () => { sockets += 1; });
  await openLocalShell(page);

  // yes writes without end. The engine takes output only as fast as it follows it on its own screen
  // (internal/terminal/screen_checkpoint.go), which a fast machine's xterm.js keeps up with; with the browser
  // slowed down, the engine gets more than its replay buffer ahead of it and skips.
  const browser = await page.context().newCDPSession(page);
  await browser.send("Emulation.setCPUThrottlingRate", { rate: 20 });
  const screen = await typeIntoTerminal(page, "yes flooding-the-terminal");
  await expect(page.getByText(/Output came faster than this window could draw it/)).toBeVisible({ timeout: 20_000 });
  await browser.send("Emulation.setCPUThrottlingRate", { rate: 1 });
  await page.keyboard.press("Control+c");
  await page.keyboard.type("echo stopped-$((6*7))");
  await page.keyboard.press("Enter");

  await expect(screen).toContainText("stopped-42", { timeout: 10_000 });
  expect(sockets).toBe(1);
});

test("moves the shell's cursor to where the line being typed is clicked", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "the prompt marks come from bash's PS1 here");
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  // Shell integration marks where the prompt and the typed text begin (OSC 133;A and B).
  const screen = await typeIntoTerminal(page, "PS1='\\[\\e]133;A\\a\\]$ \\[\\e]133;B\\a\\]'");
  await page.keyboard.type("echo 0123456789 | rev");
  // Keys typed while the shell still ran the PS1 command are echoed where the new prompt then starts,
  // so the 5 to click is the one after the prompt: the last one on the line.
  await expect(screen).toContainText("$ echo 0123456789 | rev");
  const typed = drawnSpan(page, "echo 0123456789 | rev").last();
  await expect(typed).toBeVisible();
  const box = await typed.boundingBox();
  const text = (await typed.textContent()) ?? "";
  if (box === null) throw new Error("the typed line is not drawn");
  const cellWidth = box.width / text.length;

  await page.mouse.click(box.x + (text.lastIndexOf("5") + 0.5) * cellWidth, box.y + box.height / 2);
  await page.keyboard.type("_");
  await page.keyboard.press("Enter");

  await expect(screen).toContainText("98765_43210");
});

test("saves the whole scrollback to a text file from the terminal's menu", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "seq and printf are POSIX here");
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  // More rows than "Copy recent terminal context" takes, so the file shows the start too.
  const screen = await typeIntoTerminal(page, "seq -f 'row %03g' 300; printf 'saved-%s\\n' $((6*7))");
  await expect(screen).toContainText("saved-42", { timeout: 20_000 });

  const downloaded = page.waitForEvent("download");
  await screen.getByRole("button", { name: "More terminal actions" }).click();
  await page.getByRole("menuitem", { name: "Save scrollback to a file" }).click();
  const download = await downloaded;

  expect(download.suggestedFilename()).toMatch(/^bash-\d{8}-\d{6}\.txt$/);
  const text = await readFile(await download.path(), "utf8");
  expect(text).toContain("row 001\nrow 002\n");
  expect(text).toContain("saved-42\n");
});

test("tells a program whether the terminal is shown when it asks (mode 2033)", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf and read are POSIX shell syntax here");
  await openApplication(page, installation);
  const panel = await openSessionList(page);
  await openLocalShell(page);
  // CSI ? 998 n asks once; the answer is CSI ? 999 ; 1 n while shown, 2 when not.
  const ask = "printf '\\033[?998n'; IFS= read -rs -d n reply";
  const screen = await typeIntoTerminal(page, `${ask}; printf 'ANSWER:%s\\n' "\${reply#*;}"`);
  await expect(screen).toContainText("ANSWER:1", { timeout: 20_000 });

  // The first shell asks once the second, on the screen in its place, says so; the answer comes back
  // through a file, since the first terminal is kept hidden meanwhile.
  await typeIntoTerminal(page, `until [ -e "$HOME/ask-now" ]; do sleep 0.1; done; ${ask}; printf '%s' "\${reply#*;}" > "$HOME/answer"`);
  await openLocalShell(page);
  await expect(panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem")).toHaveCount(2);
  await typeIntoTerminal(page, `touch "$HOME/ask-now"; until [ -s "$HOME/answer" ]; do sleep 0.1; done; printf 'HIDDEN:%s\\n' "$(cat "$HOME/answer")"`);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toContainText("HIDDEN:2", { timeout: 20_000 });
});

test("moves between the prompts of a long output with the prompt keys", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "the prompt marks come from bash's PS1 here");
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  // Shell integration marks where each prompt begins (OSC 133;A).
  const screen = await typeIntoTerminal(page, "PS1='\\[\\e]133;A\\a\\]$ '");
  await typeIntoTerminal(page, "echo first-$((40+2)); seq -f 'first %03g' 200");
  await expect(screen).toContainText("first 200", { timeout: 20_000 });
  await typeIntoTerminal(page, "echo second-$((40+3)); seq -f 'second %03g' 200");
  await expect(screen).toContainText("second 200", { timeout: 20_000 });

  // The top row of the screen follows the prompt it moved to.
  const topRow = drawnRows(page).locator("> div").first();
  await page.keyboard.press("Control+Shift+ArrowUp");
  await expect(topRow).toContainText("second-");
  await page.keyboard.press("Control+Shift+ArrowUp");
  await expect(topRow).toContainText("first-");
  await page.keyboard.press("Control+Shift+ArrowDown");
  await expect(topRow).toContainText("second-");
});

test("refuses to open more sessions than the configured limit", async ({ page, installation }) => {
  await installation.write(
    "sshc/metadata.json",
    standardPresetMetadata({ terminal: { maxSessions: 2, scrollbackBytes: 16384 } }),
  );
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  const openShell = panel.getByRole("button", { name: "New session" });

  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  await expect(rows).toHaveCount(1);
  await openLocalShell(page);
  await expect(rows).toHaveCount(2);

  await expect(openShell).toBeDisabled();
  await expect(panel).toContainText("limit of 2 open sessions");
});

test("shows an open session again after a reload instead of claiming there are none", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "sshc" })).toBeVisible();

  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  await expect(page.getByText("No session is open")).toBeHidden();
});

test("applies the session limit set from the settings screen", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const region = await openLoadedTerminalSettings(page);
  await region.getByLabel("Sessions open at once").fill("1");
  const saved = await saveTerminalSettings(page, region);
  expect(saved.request().postDataJSON()).toEqual(expect.objectContaining({
    preset: expect.objectContaining({ terminal: expect.objectContaining({ maxSessions: 1 }) }),
  }));
  expect(JSON.parse(await installation.read("sshc/metadata.json"))).toEqual(expect.objectContaining({
    presets: [expect.objectContaining({ id: "default", terminal: expect.objectContaining({ maxSessions: 1 }) })],
  }));

  const panel = await openSessionList(page);
  const created = page.waitForResponse((response) => {
    const request = response.request();
    return new URL(response.url()).pathname === "/api/v1/terminal/sessions" && request.method() === "POST";
  });
  const refreshed = page.waitForResponse((response) => {
    const request = response.request();
    return new URL(response.url()).pathname === "/api/v1/terminal/sessions" && request.method() === "GET";
  });
  await openLocalShell(page);
  expect((await created).status()).toBe(201);
  const listed = await refreshed;
  expect((await listed.json()).maxSessions).toBe(1);
  const terminalSessions = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await expect(terminalSessions).toHaveCount(1);

  await expect(panel.getByRole("button", { name: "New session" })).toBeDisabled();
  await expect(panel).toContainText("limit of 1 open sessions");
});

test("starts local shells where the setting says", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await installation.write("../workspace/marker", "");
  await openApplication(page, installation);

  const region = await openLoadedTerminalSettings(page);
  await openTerminalSettingsTab(region, "Behaviour");
  await region.getByLabel("Starting directory").fill("~/workspace");
  await saveTerminalSettings(page, region);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, "pwd");
  await expect(screen).toContainText(
    process.platform === "win32" ? "\\workspace" : "/workspace",
    { timeout: 20_000 },
  );
});

test("copies the selection with its colors as HTML from the terminal's menu", async ({ page, context, installation }) => {
  test.skip(process.platform === "win32", "printf is POSIX here");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, "printf '\\033[31mred-canary\\033[0m\\n'");
  await expect(screen).toContainText("red-canary", { timeout: 20_000 });

  const red = drawnSpan(page, "red-canary").last();
  const box = await red.boundingBox();
  if (box === null) throw new Error("the colored output is not drawn");
  await page.mouse.move(box.x + 1, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 1, box.y + box.height / 2, { steps: 8 });
  await page.mouse.up();
  await screen.getByRole("button", { name: "More terminal actions" }).click();
  await page.getByRole("menuitem", { name: "Copy with colors" }).click();

  const read = () => page.evaluate(async () => {
    const [first] = await navigator.clipboard.read();
    return { types: first?.types ?? [], html: first?.types.includes("text/html") ? await (await first.getType("text/html")).text() : "" };
  });
  await expect.poll(async () => (await read()).html).toContain("red-canary");
  const item = await read();
  expect(item.types).toEqual(expect.arrayContaining(["text/plain", "text/html"]));
  expect(item.html).toContain("red-canary");
  expect(item.html).toMatch(/color: ?#[0-9a-f]{6}/i);
});

test("lets a program write the clipboard through the Kitty clipboard protocol where OSC 52 is allowed", async ({ page, context, installation }) => {
  test.skip(process.platform === "win32", "printf and read are POSIX shell syntax here");
  await installation.write("sshc/metadata.json", JSON.stringify({ schemaVersion: metadataSchemaVersion, settings: { terminal: { osc52: true } } }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);

  // text/plain is dGV4dC9wbGFpbg== and kitty-canary is a2l0dHktY2FuYXJ5 in base64. The answer ends with
  // ESC \, so read stops at the backslash.
  const screen = await typeIntoTerminal(page, [
    "printf '\\033]5522;type=write\\033\\\\'",
    "printf '\\033]5522;type=wdata:mime=dGV4dC9wbGFpbg==;a2l0dHktY2FuYXJ5\\033\\\\'",
    "printf '\\033]5522;type=wdata\\033\\\\'",
    "IFS= read -rs -t 10 -d '\\' reply",
    "printf 'ANSWER:%s\\n' \"$(printf '%s' \"$reply\" | tr -dc 'a-z=:A-Z')\"",
  ].join("; "));

  await expect(screen).toContainText("ANSWER:type=write:status=DONE", { timeout: 20_000 });
  await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe("kitty-canary");
});

test("copies what was selected in the terminal as soon as selection finishes", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, "echo selectable-canary");
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

test("selects with Option on a Mac while a program reads the mouse", async ({ page, context, installation }) => {
  // xterm.js reads the platform once when it loads.
  await page.addInitScript(() => Object.defineProperty(navigator, "platform", { get: () => "MacIntel" }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  await typeIntoTerminal(page, "echo option-select-canary");
  // Turns on the mouse reports Claude Code asks for, then prints what arrives.
  const screen = await typeIntoTerminal(page, "printf '\\033[?1002h\\033[?1006h'; echo mouse-reports-$((40 + 2)); cat -v");
  await expect(screen).toContainText("mouse-reports-42", { timeout: 20_000 });

  const box = await drawnRows(page).boundingBox();
  expect(box).not.toBeNull();
  if (box === null) return;
  await page.keyboard.down("Alt");
  await page.mouse.move(box.x + 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 2, box.y + box.height - 2, { steps: 12 });
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect(selectionMarks(page).first()).toBeAttached();
  await expect
    .poll(async () => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("option-select-canary");
  await expect(screen).not.toContainText("^[[<");

  // Without Option the drag still goes to the program.
  await page.mouse.move(box.x + 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y + 2, { steps: 4 });
  await page.mouse.up();
  await expect(screen).toContainText("^[[<", { timeout: 20_000 });
});

test("sends a click to a program that asks for mouse reports without SGR", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf and cat are POSIX shell syntax here");
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  // 1000 alone asks for the default format (ESC [ M and three bytes), which
  // xterm.js reports through onBinary rather than onData.
  const screen = await typeIntoTerminal(page, "printf '\\033[?1000h'; echo default-mouse-$((40 + 2)); cat -v");
  await expect(screen).toContainText("default-mouse-42", { timeout: 20_000 });

  await terminalScreen(page).click({ position: { x: 20, y: 20 } });

  await expect(screen).toContainText("^[[M", { timeout: 20_000 });
});

test("pastes the clipboard into the terminal with right click", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.evaluate(() => navigator.clipboard.writeText("echo right-click-paste-canary"));

  await terminalScreen(page).click({ button: "right", position: { x: 20, y: 20 } });
  await page.keyboard.press("Enter");

  await expect(screen).toContainText("right-click-paste-canary", { timeout: 20_000 });
});

for (const chord of ["Control+v", "Control+Shift+V"]) {
  test(`pastes ${chord} into the terminal only once`, async ({ page, context, installation }) => {
    const inputFrames: string[] = [];
    page.on("websocket", (socket) => socket.on("framesent", ({ payload }) => {
      inputFrames.push(typeof payload === "string" ? payload : payload.toString("utf8"));
    }));
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await openApplication(page, installation);

    await openSessionList(page);
    await openLocalShell(page);
    const screen = page.getByRole("region", { name: /^Terminal for / });
    await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
    await typeIntoTerminal(page, 'rm -f "$HOME/keyboard-paste-data"');
    await page.evaluate(() => navigator.clipboard.writeText('printf x >> "$HOME/keyboard-paste-data"; '));

    await terminalKeyboard(page).focus();
    await page.keyboard.press(chord);
    await page.keyboard.press("Enter");
    const pastedFrames = inputFrames.filter((frame) => frame.includes("keyboard-paste-data"));
    expect(pastedFrames, JSON.stringify(inputFrames)).toHaveLength(1);
    expect(
      pastedFrames[0]!.split("keyboard-paste-data").length - 1,
      JSON.stringify(pastedFrames[0]),
    ).toBe(1);
    await typeIntoTerminal(page, 'echo keyboard-paste-count=$(wc -c < "$HOME/keyboard-paste-data")');

    await expect(screen).toContainText("keyboard-paste-count=1", { timeout: 20_000 });
  });
}

// Coming back to the page rechecks the vault, and the app is inert meanwhile, which takes the focus off the
// terminal. Typing right after coming back must still reach the shell, without clicking the terminal first.
test("keeps the keyboard on the terminal after the page is hidden and shown again", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, "echo before-leaving");
  await expect(screen).toContainText("before-leaving", { timeout: 20_000 });

  for (const state of ["hidden", "visible"] as const) {
    await page.evaluate((visibility) => {
      Object.defineProperty(document, "visibilityState", { configurable: true, get: () => visibility });
      document.dispatchEvent(new Event("visibilitychange"));
    }, state);
  }
  await expect(page.getByText("Checking that the vault is still unlocked before restoring protected content…")).toHaveCount(0);
  await expect(terminalKeyboard(page)).toBeFocused();
  await page.keyboard.type("echo after-coming-back");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("after-coming-back", { timeout: 20_000 });
});

test("reviews a multiline paste before sending any terminal input", async ({ page, context, installation }) => {
  const inputFrames: string[] = [];
  page.on("websocket", (socket) => socket.on("framesent", ({ payload }) => {
    inputFrames.push(typeof payload === "string" ? payload : payload.toString("utf8"));
  }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  const risky = "echo safe-paste-first\necho safe-paste-second\n";
  await page.evaluate((text) => navigator.clipboard.writeText(text), risky);

  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Shift+V");

  const dialog = page.getByRole("dialog", { name: /^Review paste to / });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Nothing has been sent to the terminal yet.");
  expect(inputFrames.filter((frame) => frame.includes("safe-paste"))).toEqual([]);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/terminal-safe-paste-review.png`,
      fullPage: true,
    });
  }

  await dialog.getByRole("button", { name: "Paste without final Enter" }).click();
  const pastedFrames = inputFrames.filter((frame) => frame.includes("safe-paste"));
  expect(pastedFrames, JSON.stringify(inputFrames)).toHaveLength(1);
  expect(pastedFrames[0]).toContain("echo safe-paste-first\recho safe-paste-second");
  expect(pastedFrames[0]).not.toContain("safe-paste-second\r");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("safe-paste-second", { timeout: 20_000 });
});

// Full-screen programs such as Claude Code redraw when the terminal loses
// focus, which the review dialog itself causes. That output must not take
// the review away from under the person reading it.
test("keeps the paste review open while a program redraws on focus changes", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  // Turns on focus reporting and prints once for every byte it reads, as a
  // redraw would.
  const screen = await typeIntoTerminal(page, "printf '\\033[?1004h'; while IFS= read -rsn1 key; do printf 'redraw-%d ' $((count += 1)); done");
  await page.evaluate(() => navigator.clipboard.writeText("echo focus-paste-first\necho focus-paste-second\n"));

  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Shift+V");

  const dialog = page.getByRole("dialog", { name: /^Review paste to / });
  await expect(dialog).toBeVisible();
  // Opening the dialog took focus from the terminal, and the program answered.
  await expect(screen).toContainText("redraw-", { timeout: 20_000 });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
});

test("keeps an ESC in a reviewed paste from ending bracketed paste and running the rest", async ({ page, context, installation }) => {
  test.skip(process.platform === "win32", "bash turns on bracketed paste here");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  // The text closes the bracket itself, so whatever follows would be typed.
  await page.evaluate(() => navigator.clipboard.writeText("echo bracket-$((1+1))\u001b[201~\necho escaped-$((2+2))\n"));

  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Shift+V");
  const dialog = page.getByRole("dialog", { name: /^Review paste to / });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Paste", exact: true }).click();
  await expect(dialog).toBeHidden();

  // bash holds the whole paste on its input line; cancel it and run a marker.
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+C");
  await page.keyboard.type("echo after-paste-$((3+3))");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("after-paste-6", { timeout: 20_000 });
  await expect(screen).not.toContainText("bracket-2");
  await expect(screen).not.toContainText("escaped-4");
});

test("edits a multiline paste before sending the changed text", async ({ page, context, installation }) => {
  const inputFrames: string[] = [];
  page.on("websocket", (socket) => socket.on("framesent", ({ payload }) => {
    inputFrames.push(typeof payload === "string" ? payload : payload.toString("utf8"));
  }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  const risky = "echo safe-paste-first\necho safe-paste-second\n";
  await page.evaluate((text) => navigator.clipboard.writeText(text), risky);

  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Shift+V");

  const dialog = page.getByRole("dialog", { name: /^Review paste to / });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Nothing has been sent to the terminal yet.");
  expect(inputFrames.filter((frame) => frame.includes("safe-paste"))).toEqual([]);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/terminal-safe-paste-review.png`,
      fullPage: true,
    });
  }

  await dialog.getByRole("textbox", { name: "Edit paste" }).fill("echo safe-paste-edited\necho safe-paste-changed\n");
  expect(inputFrames.filter((frame) => frame.includes("safe-paste"))).toEqual([]);
  await dialog.getByRole("button", { name: "Paste without final Enter" }).click();
  const pastedFrames = inputFrames.filter((frame) => frame.includes("safe-paste"));
  expect(pastedFrames, JSON.stringify(inputFrames)).toHaveLength(1);
  expect(pastedFrames[0]).toContain("echo safe-paste-edited\recho safe-paste-changed");
  expect(pastedFrames[0]).not.toContain("safe-paste-changed\r");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("safe-paste-changed", { timeout: 20_000 });
});

test("keeps terminal drawing stable while scrollback search overlays it", async ({ page, installation }) => {
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const terminal = page.getByRole("region", { name: /^Terminal for / });
  await expect(terminal).toContainText(/[$#%>]/, { timeout: 20_000 });
  await typeIntoTerminal(page, "echo search-overlay-canary");
  await expect(terminal).toContainText("search-overlay-canary", { timeout: 20_000 });
  const before = await terminalFitRects(page);

  await terminal.getByRole("button", { name: "Find" }).click();
  const searchInput = terminal.getByRole("textbox", { name: "Search terminal output" });
  await expect(searchInput).toBeVisible();
  await terminal.getByRole("button", { name: "Use regular expression" }).click();
  await searchInput.fill("search-overlay-(canary|missing)");
  await expect(searchInput.locator("..").getByRole("status")).toHaveText(/\d+\/\d+/);
  const opened = await terminalFitRects(page);
  expect(Math.abs(opened.root.y - before.root.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(opened.root.height - before.root.height)).toBeLessThanOrEqual(1);
  expect(opened.root.y + opened.root.height).toBeLessThanOrEqual(opened.host.y + opened.host.height + 1);

  await terminal.getByRole("button", { name: "Close search" }).click();
  await expect(terminal.getByRole("textbox", { name: "Search terminal output" })).toHaveCount(0);
  await expect.poll(async () => Math.abs((await terminalFitRects(page)).root.height - before.root.height)).toBeLessThanOrEqual(1);
});

test("types into the terminal again after closing the search with Escape, and offers the last query next time", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  const terminal = page.getByRole("region", { name: /^Terminal for / });
  await typeIntoTerminal(page, "echo search-focus-canary");
  await expect(terminal).toContainText("search-focus-canary", { timeout: 20_000 });

  await terminal.getByRole("button", { name: "Find" }).click();
  const searchInput = terminal.getByRole("textbox", { name: "Search terminal output" });
  await searchInput.fill("search-focus");
  await terminal.getByRole("button", { name: "Match case" }).click();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-search-bar.png` });
  }
  await expect(searchInput).toBeFocused();
  await terminal.getByRole("button", { name: "Next match" }).click();
  await expect(searchInput).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(searchInput).toHaveCount(0);

  await page.keyboard.type("echo after-search-$((5+5))");
  await page.keyboard.press("Enter");
  await expect(terminal).toContainText("after-search-10", { timeout: 20_000 });

  await terminal.getByRole("button", { name: "Find" }).click();
  await expect(terminal.getByRole("textbox", { name: "Search terminal output" })).toHaveValue("search-focus");
});

test("leaves the clipboard alone while the search selects its matches", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openSessionList(page);
  await openLocalShell(page);
  const terminal = page.getByRole("region", { name: /^Terminal for / });
  await typeIntoTerminal(page, "echo search-clipboard-canary");
  await expect(terminal).toContainText("search-clipboard-canary", { timeout: 20_000 });
  await page.evaluate(() => navigator.clipboard.writeText("copied-before-search"));

  await terminal.getByRole("button", { name: "Find" }).click();
  const searchInput = terminal.getByRole("textbox", { name: "Search terminal output" });
  await searchInput.fill("search-clipboard");
  await expect(searchInput.locator("..").getByRole("status")).toHaveText(/\d+\/\d+/);
  await terminal.getByRole("button", { name: "Next match" }).click();

  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("copied-before-search");
});

test("can turn automatic selection copy off for an already open session", async ({
  page,
  context,
  installation,
}) => {
  await seedTerminalSettings(installation);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, "echo copy-setting-canary");
  await expect(screen).toContainText("copy-setting-canary", { timeout: 20_000 });

  const settings = await openLoadedTerminalSettings(page);
  await openTerminalSettingsTab(settings, "Behaviour");
  await settings.getByRole("checkbox", { name: "Copy selected text automatically" }).uncheck();
  await saveTerminalSettings(page, settings);

  await reopenFirstSession(panel);
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

  await openSessionList(page);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();

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

test("keeps the top of the navigation still while the session list scrolls", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  const anchor = panel.getByRole("link", { name: "Connections", exact: true });
  const before = await anchor.boundingBox();

  await page.setViewportSize({ width: 1280, height: 400 });
  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  for (let opened = 1; opened <= 6; opened += 1) {
    await openLocalShell(page);
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

  const panel = await openSessionList(page);
  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await openLocalShell(page);
  await expect(rows).toHaveCount(1);
  await openLocalShell(page);
  await expect(rows).toHaveCount(2);

  await openSettingsPage(page, "Open connections");
  const region = page.getByRole("region", { name: "Open connections" });
  await expect(region.getByText("2 open")).toBeVisible();
  await region.getByRole("button", { name: "Close every connection" }).click();
  await expect(page.getByRole("dialog").getByRole("list", { name: "Sessions that end" }).getByRole("listitem")).toHaveCount(2);
  await page.getByRole("button", { name: "Close them all" }).click();

  await expect(region.getByText("0 open")).toBeVisible();
  await openSessionList(page);
  await expect(rows).toHaveCount(0);
});

test("force closes a live local shell with one confirmation", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await openLocalShell(page);
  await expect(rows).toHaveCount(1);

  await panel.getByRole("button", { name: /^Close / }).click();
  const closeResponse = page.waitForResponse((response) => {
    const request = response.request();
    return request.method() === "DELETE" && /\/api\/v1\/terminal\/sessions\/[^/]+$/.test(new URL(response.url()).pathname);
  });
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Keep it open" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: "Close", exact: true })).toBeFocused();
  await page.keyboard.press("Enter");
  expect((await closeResponse).ok()).toBe(true);

  await expect(rows).toHaveCount(0);
});

test("previews and closes a live connection immediately while Shift is held", async ({ page, installation }) => {
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  const rows = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await openLocalShell(page);
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
  await tree.getByRole("button", { name: "detail-host", exact: true }).click();
  const detail = page.getByRole("heading", { name: "detail-host" });
  await expect(detail).toBeVisible();

  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await expect(page).toHaveURL(/\/terminal$/);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  await expect(tree).toBeHidden();

  await page.goBack();
  await expect(page).not.toHaveURL(/\/terminal$/);
  await expect(detail).toBeVisible();
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeHidden();
});

test("shows why a connection failed in the terminal itself", async ({ page, installation }) => {
  await installation.write(
    "conf.d/20-refused.conf",
    ["Host refused", "\tHostName 127.0.0.1", "\tPort 1", "\tConnectTimeout 2", ""].join("\n"),
  );
  await openApplication(page, installation);

  await openSection(page, "Connections");
  const nav = page.getByRole("navigation", { name: "Connections" });
  await nav.getByRole("button", { name: "refused", exact: true }).click();
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/sshc:.*(refused|connect)/i, { timeout: 20_000 });
});

test("tells the pseudo-terminal how big it is as soon as it attaches", async ({ page, installation }) => {
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, shellSays.size);
  await expect(screen).toContainText(/\d+-\d+/, { timeout: 20_000 });

  const reported = (await screen.innerText()).match(/(\d+)-(\d+)/);
  expect(reported).not.toBeNull();
  const drawn = await drawnRowCount(page);
  expect(Number(reported?.[1])).toBe(drawn);
  expect(Number(reported?.[2])).toBeGreaterThan(80);
});

test("keeps the same terminal alive while another screen is shown", async ({ page, installation }) => {
  await openApplication(page, installation);

  await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, "echo stays-mounted");
  await expect(screen).toContainText("stays-mounted", { timeout: 20_000 });

  const sameTerminal = await markTerminal(page, "1");
  await typeIntoTerminal(page, shellSays.lateEcho("late-42"));

  await openSettingsPage(page, "General");
  await expect(screen).toBeHidden();

  const reopened = await openSessionList(page);
  await reopened
    .getByRole("list", { name: "Open sessions" })
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

  await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, 'echo "prefix=[${npm_config_prefix}]"');

  await expect(screen).toContainText("prefix=[]", { timeout: 20_000 });
});

test("paints the terminal in the colour scheme that was chosen", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  await openLocalShell(page);
  const screen = await typeIntoTerminal(page, shellSays.redWord("zzred"));
  await expect(screen).toContainText("zzred", { timeout: 20_000 });

  const surface = () => surfaceToken(page, "--ui-term-bg");
  const drawnRed = () =>
    drawnSpan(page, "zzred")
      .last()
      .evaluate((node) => getComputedStyle(node as HTMLElement).color);

  const beforeSurface = await surface();
  const beforeRed = await drawnRed();

  const settings = await openLoadedTerminalSettings(page);
  await openTerminalSettingsTab(settings, "Display");
  await settings.getByLabel("Colour scheme").selectOption("dracula");
  await saveTerminalSettings(page, settings);

  await reopenFirstSession(panel);
  await expect.poll(surface).toBe("#282a36");
  await expect.poll(drawnRed).toBe("rgb(255, 85, 85)");
  expect(beforeSurface).not.toBe("#282a36");
  expect(beforeRed).not.toBe("rgb(255, 85, 85)");
});

test("loads the font it ships and hands it to the terminal", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();

  const family = async () => (await drawnRowFont(page)).fontFamily;
  expect(await family()).not.toContain("JetBrains Mono");

  const settings = await openLoadedTerminalSettings(page);
  await openTerminalSettingsTab(settings, "Display");
  await settings.getByLabel("Font family").selectOption("jetbrains-mono");
  await saveTerminalSettings(page, settings);

  await reopenFirstSession(panel);
  await expect.poll(family).toContain("JetBrains Mono");
  await expect
    .poll(async () => page.evaluate(() => document.fonts.check('13px "JetBrains Mono"')))
    .toBe(true);
});

test("wears the image that was brought in, and gets out of its way", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);

  const panel = await openSessionList(page);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();

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
  await openTerminalSettingsTab(settings, "Display");
  await settings.getByRole("button", { name: "Change", exact: true }).click();
  const library = page.getByRole("dialog", { name: "Background image library" });
  await library.locator('input[type="file"]').setInputFiles({
    name: "Canary Wall.png",
    mimeType: "image/png",
    buffer: Buffer.from(encoded.split(",")[1] ?? "", "base64"),
  });
  const thumbnail = library.getByRole("img", { name: "canary-wall.png" });
  await expect(thumbnail).toBeVisible();
  await expect.poll(async () => thumbnail.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
  await library.getByRole("button", { name: "Use this image", exact: true }).click();
  await saveTerminalSettings(page, settings);

  await reopenFirstSession(panel);
  const wiring = {
    image: await surfaceBackgroundImage(page),
    viewport: await viewportBackground(page),
  };
  expect(wiring.image).toContain("data:image/png");
  expect(wiring.image).toMatch(/^linear-gradient\(/);
  expect(wiring.viewport).toBe("rgba(0, 0, 0, 0)");

  const cleared = await openLoadedTerminalSettings(page);
  await openTerminalSettingsTab(cleared, "Display");
  await cleared.getByRole("button", { name: "Clear", exact: true }).click();
  const saved = await saveTerminalSettings(page, cleared);
  expect(saved.request().postDataJSON()).not.toHaveProperty("appearance");
  // The settings page says a change was saved beside its name, for every page alike.
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await reopenFirstSession(panel);
  await expect.poll(() => surfaceBackgroundImage(page)).toBe("none");
});


test("selects only the visible screen and opens a session from row padding", async ({ page, installation }) => {
  await openApplication(page, installation);
  const panel = await openSessionList(page);
  await openLocalShell(page);
  const row = panel.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first();
  await expect(row.locator("[aria-current=true]")).toHaveCount(1);
  for (const destination of ["Home", "SFTP"]) {
    await openSection(page, destination);
    await expect(row.locator("[aria-current]")).toHaveCount(0);
    // Bottom-right padding, below the actual action buttons, belongs to the row.
    const box = await row.boundingBox();
    if (box === null) throw new Error("missing session row");
    await page.mouse.click(box.x + box.width - 3, box.y + box.height - 3);
    await expect(row.locator("[aria-current=true]")).toHaveCount(1);
    await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  }
  // The actual close button must remain independently clickable.
  await row.getByRole("button", { name: /^Close / }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "Keep it open" }).click();
});

test("expands the transfer manager from its empty header space", async ({ page, installation }) => {
  // The transfer manager starts folded.
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const expand = page.getByRole("button", { name: "Expand transfer manager" });
  await expect(expand).toBeVisible();
  const header = expand.locator("..");
  const box = await header.boundingBox();
  if (box === null) throw new Error("missing transfer header");
  await page.mouse.click(box.x + box.width * 0.75, box.y + box.height / 2);
  await expect(page.getByRole("button", { name: "Collapse transfer manager" })).toHaveAttribute("aria-expanded", "true");
});

// A preset holds how terminals look and behave on the machines that use it. A machine that uses it draws its
// terminals with it, and the standard preset stays as it was for the machines that do not.
test("draws the terminals with the preset this machine uses", async ({ page, installation }) => {
  await seedTerminalSettings(installation);
  await openApplication(page, installation);
  const region = await openLoadedTerminalSettings(page);
  await openTerminalSettingsTab(region, "Display");

  await page.getByRole("button", { name: "Actions for Standard" }).click();
  await page.getByRole("menuitem", { name: "New preset" }).click();
  const naming = page.getByRole("dialog", { name: "New preset from Standard" });
  await naming.getByRole("textbox", { name: "Preset name" }).fill("Large");
  await naming.getByRole("button", { name: "New preset" }).click();
  await expect(naming).toBeHidden();
  await expect(page.getByRole("combobox", { name: "Preset to edit" }).locator("option:checked")).toHaveText("Large");

  await openTerminalSettingsTab(region, "Display");
  await region.getByLabel("Font size").fill("22");
  const presetSave = page.waitForResponse((response) =>
    new URL(response.url()).pathname.startsWith("/api/v1/settings/presets/") && response.request().method() === "PUT");
  // Enter confirms a typed field as leaving it does.
  await region.getByLabel("Font size").press("Enter");
  expect((await presetSave).ok()).toBe(true);
  await page.getByRole("button", { name: "Use on this machine" }).click();
  await expect(page.getByText("In use here")).toBeVisible();

  await openSessionList(page);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  await expect.poll(async () => (await drawnRowFont(page)).fontSize).toBe("22px");

  // The standard preset kept its size; the new one's lives in that preset alone.
  const metadata = JSON.parse(await installation.read("sshc/metadata.json")) as {
    presets: { id: string; name?: string; terminal?: { fontSize?: number } }[];
  };
  expect(metadata.presets[0]?.terminal?.fontSize).toBeUndefined();
  expect(metadata.presets.slice(1)).toEqual([expect.objectContaining({ name: "Large", terminal: expect.objectContaining({ fontSize: 22 }) })]);
});

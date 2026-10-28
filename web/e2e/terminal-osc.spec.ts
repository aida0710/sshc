import { expect, openApplication, openSection, test, openLocalShell } from "./support/environment";
import { drawnSpan, terminalKeyboard, waitForTerminalLayout } from "./support/terminal";

test("does not repeat an old clipboard request when reopening a terminal session", async ({ page, context, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/);
  await screen.getByRole("button", { name: "More terminal actions" }).click();
  await screen.getByRole("menuitemcheckbox", { name: "OSC 52" }).click();

  const copiedText = "copied-before-switch";
  const copyCommand = `printf '\\033]52;c;${Buffer.from(copiedText).toString("base64")}\\a'`;
  await terminalKeyboard(page).focus();
  await page.keyboard.type(copyCommand);
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(copiedText);
  await expect(screen).toContainText("The remote application copied text to the system clipboard.");

  await openLocalShell(page);
  await expect(page.getByRole("list", { name: "Open sessions" }).getByRole("listitem")).toHaveCount(2);
  await page.evaluate(() => navigator.clipboard.writeText("clipboard-after-switch"));
  await page.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first().getByRole("button").first().click();
  await expect(screen).toContainText(copyCommand);
  await waitForTerminalLayout(page);
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("clipboard-after-switch");
  await expect(screen).not.toContainText("The remote application copied text to the system clipboard.");

  // Sending the same text again is a new copy request and must still work.
  await terminalKeyboard(page).focus();
  await page.keyboard.type(copyCommand);
  await page.keyboard.press("Enter");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(copiedText);
});

// Programs rename panes with OSC 0/1/2 and raise notifications with
// OSC 9/99/777. The engine records both, so the pane header, the session
// list and the unread marks follow without any agent-specific plugin.
test("applies terminal titles to the pane and marks notifications unread", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const nav = page.getByRole("navigation", { name: "Primary" });
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();

  // `read` keeps the program in the foreground so a prompt that sets its own
  // title cannot immediately replace ours.
  await page.keyboard.type("printf '\\033]0;deploy checklist\\a'; read -r _");
  await page.keyboard.press("Enter");
  const terminalSessions = nav.getByRole("list", { name: "Open sessions" });
  await expect(page.getByRole("region", { name: "Terminal for deploy checklist" })).toBeVisible({ timeout: 15_000 });
  await expect(terminalSessions.getByRole("listitem").first()).toContainText("deploy checklist");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-osc-title.png`, fullPage: true });
  }
  await page.keyboard.press("Enter");

  // Ask for a notification after a delay, then leave the pane. The unread
  // mark must appear on the pane we are no longer looking at.
  await page.keyboard.type("sleep 3; printf '\\033]777;notify;Build finished;main.go compiled\\a'");
  await page.keyboard.press("Enter");
  await openLocalShell(page);
  await expect(terminalSessions.getByRole("listitem")).toHaveCount(2);
  const first = terminalSessions.getByRole("listitem").first();
  await expect(first.getByText(/Build finished\s+main\.go compiled/)).toBeVisible({ timeout: 20_000 });
  await expect(first.getByRole("button").first()).toHaveAccessibleDescription(/Unread notification/);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-osc-unread.png`, fullPage: true });
  }

  // Showing the pane again reads the notification.
  await first.getByRole("button").first().click();
  await expect(first.getByText(/Build finished/)).toHaveCount(0);
  await expect(first.getByRole("button").first()).not.toHaveAccessibleDescription(/Unread notification/);
});

test("turns OSC 52 on for every terminal from one terminal's menu", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  const first = page.getByRole("region", { name: /^Terminal for / });
  await expect(first).toContainText(/[$#%>]/);
  await first.getByRole("button", { name: "More terminal actions" }).click();
  await first.getByRole("group", { name: "Global settings" }).getByRole("menuitemcheckbox", { name: "OSC 52" }).click();
  await expect(first).toContainText("OSC 52 clipboard writes are now allowed in every terminal.");

  await openLocalShell(page);
  await expect(page.getByRole("list", { name: "Open sessions" }).getByRole("listitem")).toHaveCount(2);
  const second = page.getByRole("region", { name: /^Terminal for / });
  await expect(second).toContainText(/[$#%>]/);
  await second.getByRole("button", { name: "More terminal actions" }).click();
  await expect(second.getByRole("menuitemcheckbox", { name: "OSC 52" })).toHaveAttribute("aria-checked", "true");
});

test("shows where an OSC 8 link after Japanese text goes before opening it, unless its text is that address", async ({ page, context, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/);
  const opened: string[] = [];
  context.on("page", (popup) => opened.push(popup.url()));

  // Two-cell characters before the link shift a string slice of the row. The
  // shell here has no UTF-8 locale, so the Japanese text is typed as octal bytes.
  const octal = (text: string) => [...Buffer.from(text, "utf8")].map((byte) => byte < 0x80 ? String.fromCharCode(byte) : `\\${byte.toString(8)}`).join("");
  const link = (target: string, text: string) => `\\033]8;;${target}\\033\\\\${octal(text)}\\033]8;;\\033\\\\`;
  const prefix = octal("日本語の説明 ");
  await terminalKeyboard(page).focus();
  await page.keyboard.type(`printf '${prefix}${link("https://example.test/docs", "こちら")}\\n${prefix}${link("https://example.test/same", "https://example.test/same")}\\n'`);
  await page.keyboard.press("Enter");
  const label = drawnSpan(page, "こちら").last();
  await expect(label).toBeVisible();

  // The spans sit under xterm's screen layer, which takes the pointer, so
  // the click goes to where the text is drawn.
  const controlClick = async (target: typeof label) => {
    const box = await target.boundingBox();
    if (box === null) throw new Error("the link text is not drawn");
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.keyboard.down("Control");
    await page.mouse.down();
    await page.mouse.up();
    await page.keyboard.up("Control");
  };
  await controlClick(label);
  const actions = page.getByRole("dialog", { name: "Terminal link actions" });
  await expect(actions).toContainText("https://example.test/docs");
  expect(opened).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(actions).toBeHidden();

  const address = drawnSpan(page, "https://example.test/same").last();
  const popup = context.waitForEvent("page");
  await controlClick(address);
  await popup;
  await expect(actions).toBeHidden();
});

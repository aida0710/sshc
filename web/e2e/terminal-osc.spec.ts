import type { Page } from "@playwright/test";
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
  await first.getByRole("group", { name: "All machines" }).getByRole("menuitemcheckbox", { name: "OSC 52" }).click();
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

// recordBrowserNotifications replaces the browser's Notification API, which a headless browser refuses, so a test can
// read what sshc showed and answer its request for permission.
async function recordBrowserNotifications(page: Page, permission: NotificationPermission) {
  await page.addInitScript((initialPermission) => {
    const shown: { title: string; body: string; closed: boolean }[] = [];
    class RecordedNotification {
      static permission = initialPermission;
      static requestPermission = async () => {
        RecordedNotification.permission = "granted";
        return "granted";
      };
      private record: { title: string; body: string; closed: boolean };
      constructor(title: string, options: { body: string }) {
        this.record = { title, body: options.body, closed: false };
        shown.push(this.record);
      }
      close() {
        this.record.closed = true;
      }
    }
    Object.assign(window, { Notification: RecordedNotification, shownNotifications: shown });
  }, permission);
  return () => page.evaluate(() => (window as unknown as { shownNotifications: { title: string; body: string; closed: boolean }[] }).shownNotifications);
}

const reportProgramStatus = (fields: string) => `printf '\\033]7501;${fields}\\033\\\\'`;

// A person away from the tab hears that a program waits for them, and that is taken back once it stops waiting.
test("a browser notifies a person who is away that a program waits for them", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  const notifications = await recordBrowserNotifications(page, "granted");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await page.evaluate(() => { document.hasFocus = () => false; });
  await terminalKeyboard(page).focus();
  await page.keyboard.type(`printf '\\033]0;agent\\a'; ${reportProgramStatus(`state=blocked:kind=permission:msg=${Buffer.from("Run rm -rf build?").toString("base64")}`)}; sleep 2; ${reportProgramStatus("state=working")}`);
  await page.keyboard.press("Enter");

  await expect.poll(notifications, { timeout: 15_000 }).toEqual([expect.objectContaining({ body: expect.stringContaining("Run rm -rf build?"), closed: false })]);
  await expect.poll(notifications, { timeout: 15_000 }).toEqual([expect.objectContaining({ closed: true })]);
});

// Nothing can reach a person who never allowed notifications, so sshc asks the first time it matters, and only once.
test("a browser asks once for permission to notify when a program first says what it is doing", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await recordBrowserNotifications(page, "default");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  const question = screen.getByText("Get notified when a program is waiting?");
  await expect(question).toHaveCount(0);
  await terminalKeyboard(page).focus();
  await page.keyboard.type(reportProgramStatus("state=working"));
  await page.keyboard.press("Enter");

  await expect(question).toBeVisible({ timeout: 15_000 });
  await expect(screen).toContainText("your browser notifies you while you are away from this tab");
  if (process.env.SSHC_NOTIFICATION_NOTICE_SHOT) await page.screenshot({ path: process.env.SSHC_NOTIFICATION_NOTICE_SHOT });
  await screen.getByRole("button", { name: "Enable notifications", exact: true }).click();
  await expect(question).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => Notification.permission)).toBe("granted");
});

// The list of sessions is read every two seconds. A program's row follows what the program says sooner than that:
// the ring turns as soon as the program works and stops as soon as it does, as the program's own screen shows it.
test("a session's row follows a program that works for less than the time between two reads of the list", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  const sessions = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" });
  // The rows are watched from before the program starts, so no ring that came and went is missed.
  await sessions.evaluate((list) => {
    const counted = window as unknown as { ringsShown: number };
    counted.ringsShown = 0;
    new MutationObserver((changes) => {
      for (const change of changes) {
        const mark = change.target as Element;
        if (change.type === "attributes" && mark.getAttribute("data-program-status") === "working") counted.ringsShown += 1;
        for (const added of change.addedNodes) {
          if (added instanceof Element && added.matches('[data-program-status="working"]')) counted.ringsShown += 1;
        }
      }
    }).observe(list, { subtree: true, childList: true, attributes: true, attributeFilter: ["data-program-status"] });
  });
  const timesWorked = 4;
  await terminalKeyboard(page).focus();
  // Each spell of work and each rest is 0.7s. Read every 2s, the list would catch three spells at most.
  await page.keyboard.type(`for spell in 1 2 3 4; do ${reportProgramStatus("state=working")}; sleep 0.7; ${reportProgramStatus("state=idle")}; sleep 0.7; done; echo spells-$((2+2))-over`);
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("spells-4-over", { timeout: 20_000 });

  await expect.poll(() => page.evaluate(() => (window as unknown as { ringsShown: number }).ringsShown)).toBe(timesWorked);
  await expect(sessions.locator("[data-program-status]")).toHaveCount(0);
});

// Two programs that began working at different moments turn their rings together. Left to start when each appears,
// one ring points up while the other points down.
test("the rings of two working programs turn in step, whenever each began", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await openApplication(page, installation);
  const sessions = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" });
  const screen = page.getByRole("region", { name: /^Terminal for / });
  // Both sessions are open before either program works, so a ring appears in a list that is already drawn.
  await openLocalShell(page);
  await openLocalShell(page);
  await expect(sessions.getByRole("listitem")).toHaveCount(2);
  const rings = sessions.locator('[data-program-status="working"]');
  for (const row of [1, 0]) {
    await sessions.getByRole("listitem").nth(row).getByRole("button").first().click();
    await expect(screen).not.toContainText("read -r");
    await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
    await terminalKeyboard(page).focus();
    await page.keyboard.type(`${reportProgramStatus("state=working")}; read -r _`);
    await page.keyboard.press("Enter");
    await expect(rings).toHaveCount(2 - row, { timeout: 15_000 });
    // The second program begins a part of a turn after the first.
    await page.waitForTimeout(370);
  }

  // Both angles are read in one step, so the rings have no time to turn between the two readings.
  const degrees = await rings.evaluateAll((marks) => marks.map((mark) => {
    const turn = new DOMMatrixReadOnly(getComputedStyle(mark).transform);
    return (Math.atan2(turn.b, turn.a) * 180) / Math.PI;
  }));
  const apart = Math.abs(((degrees[0]! - degrees[1]! + 540) % 360) - 180);
  expect(apart, `the rings stand at ${degrees.map((angle) => angle.toFixed(0)).join("° and ")}°`).toBeLessThan(2);
});

// Programs say what they are doing with OSC 7501. The session's row shows it in
// place of its dot and destination: working and blocked while they last, a done
// result until the session has been on screen since it came.
test("shows what a program says it is doing in its session's row until the result is seen", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  const report = (fields: string) => `printf '\\033]7501;${fields}\\033\\\\'`;
  const message = (text: string) => Buffer.from(text).toString("base64");
  await terminalKeyboard(page).focus();
  // The result comes after a pause, while another session is on screen.
  await page.keyboard.type(`printf '\\033]0;agent\\a'; ${report(`state=blocked:kind=permission:msg=${message("Run rm -rf build?")}`)}; sleep 8; ${report(`state=done:msg=${message("Docs built")}`)}; read -r _`);
  await page.keyboard.press("Enter");

  const sessions = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" });
  const agent = sessions.getByRole("button", { name: "agent", exact: true });
  await expect(agent).toHaveAccessibleDescription("Needs permission · Run rm -rf build?", { timeout: 15_000 });
  await expect(agent.locator("[data-program-status]")).toHaveAttribute("data-program-status", "blocked");

  await openLocalShell(page);
  await expect(agent).toHaveAccessibleDescription("Done · Docs built", { timeout: 20_000 });
  await expect(agent.locator("[data-program-status]")).toHaveAttribute("data-program-status", "done");
  const mark = agent.locator(":scope > span[aria-hidden]");
  const name = agent.getByTitle("agent", { exact: true });
  const doneMark = await mark.boundingBox();
  const doneName = await name.boundingBox();

  await agent.click();
  await expect(agent).toHaveAccessibleDescription("connected");
  await expect(agent.locator("[data-program-status]")).toHaveCount(0);
  // The connection dot is as large as the program's mark it comes back from, so the name stays where it was.
  const dot = await mark.boundingBox();
  expect({ width: dot?.width, height: dot?.height }).toEqual({ width: doneMark?.width, height: doneMark?.height });
  expect((await name.boundingBox())?.x).toBe(doneName?.x);
});

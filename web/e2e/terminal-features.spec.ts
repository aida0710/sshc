import { join } from "node:path";
import { expect, openApplication, openSection, openSettingsPage, test, openLocalShell } from "./support/environment";
import { terminalKeyboard, terminalScrollbarSlider } from "./support/terminal";

const visualDirectory = process.env.SSHC_VISUAL_DIR;

async function mockConnectedTerminal(
  page: import("@playwright/test").Page,
  session: Record<string, unknown>,
  output: string,
) {
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ streamTicket: String(session.id) }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ sessions: [session], maxSessions: 50 }),
    });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(Buffer.from(output));
  });
}

test("opens and copies a complete URL from its wrapped continuation", async ({ page, installation, context }) => {
  const url = `https://example.test/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%2Fcallback&state=${"a".repeat(240)}`;
  await page.setViewportSize({ width: 800, height: 720 });
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.addInitScript(() => {
    window.open = (target) => {
      document.documentElement.dataset.openedUrl = String(target);
      return null;
    };
  });
  await mockConnectedTerminal(page, {
    id: "wrapped-link", kind: "shell", title: "sso-login", startedAt: "2026-09-26T00:00:00Z",
    state: "connected", problem: "", forwards: [],
  }, `Open this URL to sign in:\r\n${url}\r\n`);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const continuation = page.locator(".xterm-rows > div").filter({ hasText: /^a{20,}$/ }).first();
  await expect(continuation).toBeVisible();
  const rowBounds = await continuation.boundingBox();
  if (rowBounds === null) throw new Error("the wrapped row is not visible");
  // xterm's screen receives pointer events above its DOM text rows.
  const clickPosition = { x: rowBounds.x + 25, y: rowBounds.y + rowBounds.height / 2 };
  await page.mouse.click(clickPosition.x, clickPosition.y);
  const actions = page.getByRole("dialog", { name: "Terminal link actions" });
  await expect(actions.locator("p")).toHaveAttribute("title", url);
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-wrapped-url.png"), fullPage: true });
  }
  await actions.getByRole("button", { name: "Copy link" }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(url);
  await expect(actions).toBeHidden();
  // Re-enter at another cell so xterm refreshes hover after the popover closes.
  const directClickX = clickPosition.x + 30;
  await page.mouse.move(directClickX, clickPosition.y);
  await expect(page.locator(".xterm-screen")).toHaveClass(/xterm-cursor-pointer/);
  await page.keyboard.down("Control");
  await page.mouse.click(directClickX, clickPosition.y);
  await page.keyboard.up("Control");
  await expect(page.locator("html")).toHaveAttribute("data-opened-url", url);
});

test("renders the documented non-interactive CLI example", async ({ page, installation }) => {
  const session = {
    id: "cli-production",
    kind: "ssh",
    alias: "production",
    title: "production",
    startedAt: "2026-09-01T00:00:00Z",
    state: "connected",
    problem: "",
    forwards: [],
  };
  await mockConnectedTerminal(
    page,
    session,
    "$ sshc ssh production --non-interactive -- uname -n\r\nproduction\r\n$ ",
  );

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const terminal = page.getByRole("region", { name: "Terminal for production" });
  await expect(terminal).toContainText("sshc ssh production --non-interactive -- uname -n");
  await expect(terminal).toContainText("production");

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "cli-example-desktop.png"), fullPage: true });
  }
});

test("uses a thin rounded scrollbar for terminal scrollback", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await openLocalShell(page);

  const terminal = page.getByRole("region", { name: /^Terminal for / });
  await expect(terminal).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type("i=1; while [ \"$i\" -le 120 ]; do printf 'scrollback-line-%03d\\n' \"$i\"; i=$((i+1)); done");
  await page.keyboard.press("Enter");
  await expect(terminal).toContainText("scrollback-line-120", { timeout: 20_000 });

  await terminal.hover();
  await page.mouse.wheel(0, -800);

  const slider = terminalScrollbarSlider(page);
  await expect(slider).toBeVisible();
  const scrollbar = await slider.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      width: style.width,
      radius: style.borderRadius,
    };
  });
  expect(scrollbar).toEqual({
    width: "6px",
    radius: "999px",
  });

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-modern-scrollbar-desktop.png"), fullPage: true });
  }
});

test("keeps terminal actions compact and exposes terminal settings", async ({ page, installation }) => {
  await mockConnectedTerminal(
    page,
    {
      id: "terminal-actions",
      kind: "shell",
      title: "docs-shell",
      startedAt: "2026-09-01T00:00:00Z",
      state: "connected",
      problem: "",
      forwards: [],
    },
    "developer@workstation:~$ ",
  );
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const terminal = page.getByRole("region", { name: "Terminal for docs-shell" });
  await expect(terminal).toBeVisible();
  await expect(terminal.getByRole("button", { name: "Find" })).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  const menu = terminal.getByRole("menu", { name: "More terminal actions" });
  await expect(menu.getByRole("menuitem", { name: "Quick Commands" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Copy recent terminal context" })).toBeVisible();
  await expect(menu.getByRole("menuitemcheckbox", { name: "OSC 52" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Port forwarding" })).toHaveCount(0);

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-actions-desktop.png"), fullPage: true });
  }

  await page.keyboard.press("Escape");
  await openSettingsPage(page, "Terminal");
  const settings = page.getByRole("region", { name: "Terminal" });
  await expect(settings.getByLabel("Browser scrollback (lines)")).toBeVisible();
  await expect(settings.getByLabel("Default local shell")).toBeVisible();
  await expect(settings.getByText("Allow OSC 52 clipboard writes by default")).toBeVisible();
  await expect(settings.getByText("Send the JIS ¥ key as backslash")).toBeVisible();

  if (visualDirectory !== undefined) {
    await settings.getByLabel("Browser scrollback (lines)").scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(visualDirectory, "terminal-settings-desktop.png"), fullPage: true });
    await settings.getByText("Send the JIS ¥ key as backslash").scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(visualDirectory, "terminal-input-settings-desktop.png"), fullPage: true });
  }

  await page.setViewportSize({ width: 360, height: 800 });
  await page.reload();
  await openSection(page, "Terminal");
  await expect(terminal).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  await expect(menu).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-actions-mobile.png"), fullPage: true });
  }
});

test("opens forwarding management only for an SSH terminal", async ({ page, installation }) => {
  const session = {
    id: "forwarding-preview",
    kind: "ssh",
    alias: "bastion",
    title: "bastion",
    startedAt: "2026-08-30T00:00:00Z",
    state: "connected",
    problem: "",
    forwards: [{
      id: "pf-1",
      kind: "dynamic",
      listen: "127.0.0.1:1080",
      to: "",
      problem: "",
      temporary: true,
    }],
  };
  await page.route("**/api/v1/terminal/sessions", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 50 }) });
      return;
    }
    await route.continue();
  });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const terminal = page.getByRole("region", { name: "Terminal for bastion" });
  await expect(terminal).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  await terminal.getByRole("menuitem", { name: "Port forwarding" }).click();
  const dialog = page.getByRole("dialog", { name: "Port forwarding" });
  await expect(dialog.getByText("socks5://127.0.0.1:1080")).toBeVisible();
  await expect(dialog.getByText(/Listeners are bound to this device only/)).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "port-forwarding-live-desktop.png"), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: join(visualDirectory, "port-forwarding-live-mobile.png"), fullPage: true });
  }
});

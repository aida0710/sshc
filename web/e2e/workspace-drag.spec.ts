import { expect, openApplication, openSection, test } from "./support/environment";

const sessions = [
  {
    id: "workspace-edge",
    kind: "ssh",
    alias: "edge",
    title: "edge",
    startedAt: "2026-08-26T08:00:00Z",
    state: "connected",
    problem: "",
    forwards: [],
  },
  {
    id: "workspace-database",
    kind: "ssh",
    alias: "database",
    title: "database",
    startedAt: "2026-08-26T08:01:00Z",
    state: "connected",
    problem: "",
    forwards: [],
  },
];

test("docks connected terminals into a live workspace", async ({ page, installation }) => {
  let ticket = 0;
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/stream")) {
      ticket += 1;
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ streamTicket: `workspace-${ticket}` }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ sessions, maxSessions: 12 }),
    });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    const current = new URL(socket.url()).searchParams.get("ticket") ?? "terminal";
    socket.send(Buffer.from(`[sshc] ${current} connected\r\n$ `));
  });

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("tab", { name: "Terminals" }).click();
  await navigation.getByRole("button", { name: "edge", exact: true }).click();

  const target = page.locator("[data-single-terminal-drop-target='workspace-edge']");
  await expect(target).toBeVisible();
  const databaseRow = navigation
    .getByRole("list", { name: "Open consoles" })
    .getByRole("listitem")
    .filter({ hasText: "connected · database" });
  const targetBox = await target.boundingBox();
  expect(targetBox).not.toBeNull();
  await databaseRow.dragTo(target, {
    targetPosition: { x: Math.max(1, targetBox!.width - 8), y: targetBox!.height / 2 },
  });

  await expect(page.locator("[data-workspace-pane]")).toHaveCount(2);
  await expect(page.locator("[data-pane-toolbar]")).toHaveCount(2);
  await expect(navigation.getByRole("button", { name: "edge + database", exact: true })).toBeVisible();
  await expect(navigation.getByText("2 terminals", { exact: true })).toBeVisible();

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.0-live-workspace-desktop.png`, fullPage: true });
  }

  await page.getByRole("button", { name: "Send command…" }).click();
  const broadcast = page.getByRole("dialog", { name: "Send to connected terminals" });
  await expect(broadcast).toBeVisible();
  await expect(broadcast.getByText(/working directory, environment, and shell state/)).toBeVisible();
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.0-broadcast-modal.png`, fullPage: true });
  }
  await broadcast.getByRole("button", { name: "Close command delivery" }).click();

  const savedLayouts = page.locator("summary").filter({ hasText: "Saved layouts" });
  await savedLayouts.click();
  await expect(page.getByText(/Save SSH targets, local shells, and split ratios/)).toBeVisible();
  if (visualDirectory !== undefined) {
    await expect(savedLayouts.locator("[aria-hidden='true']")).toHaveText("›");
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-saved-layouts.png`, fullPage: true });
  }
  await savedLayouts.click();

  await page.setViewportSize({ width: 360, height: 800 });
  await page.waitForTimeout(400);
  await expect(page.getByRole("navigation", { name: "Workspace terminals" })).toBeVisible();
  await expect(page.locator("[data-workspace-pane]")).toHaveCount(1);
  await expect(page.locator("[data-pane-toolbar]")).toHaveCount(0);
  await expect(page.locator("[data-desktop-workspace-controls]")).toBeHidden();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.0-live-workspace-mobile.png`, fullPage: true });
  }
});

test("selects the whole local console row and docks local shells", async ({ page, installation }) => {
  const localSessions = [
    {
      id: "local-zsh",
      kind: "shell",
      title: "zsh",
      startedAt: "2026-08-27T03:00:00Z",
      state: "connected",
      problem: "",
      forwards: [],
    },
    {
      id: "local-bash",
      kind: "shell",
      title: "bash",
      startedAt: "2026-08-27T03:01:00Z",
      state: "connected",
      problem: "",
      forwards: [],
    },
  ];
  let ticket = 0;
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      ticket += 1;
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ streamTicket: `local-${ticket}` }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ sessions: localSessions, maxSessions: 12 }),
    });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(Buffer.from("local shell\r\n$ "));
  });

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("tab", { name: "Terminals" }).click();
  const consoleList = navigation.getByRole("list", { name: "Open consoles" });
  const zshRow = consoleList.getByRole("listitem").filter({ hasText: "zsh" });
  await zshRow.getByText("connected · localhost").click();

  const target = page.locator("[data-single-terminal-drop-target='local-zsh']");
  await expect(target).toBeVisible();
  const bashRow = consoleList.getByRole("listitem").filter({ hasText: "bash" });
  const targetBox = await target.boundingBox();
  expect(targetBox).not.toBeNull();
  await bashRow.dragTo(target, {
    targetPosition: { x: Math.max(1, targetBox!.width - 8), y: targetBox!.height / 2 },
  });

  await expect(page.locator("[data-workspace-pane]")).toHaveCount(2);
  await expect(page.locator("[data-pane-toolbar]")).toHaveCount(2);
  await expect(page.getByRole("button", { name: "Send command…" })).toBeDisabled();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-local-shell-workspace.png`,
      fullPage: true,
    });
  }
});

import { expect, openApplication, openSection, test } from "./support/environment";

const exited = {
  id: "manual-reconnect-fixture",
  kind: "ssh", workspaceId: "default",
  alias: "production-api",
  title: "production-api",
  startedAt: "2026-08-26T01:00:00Z",
  state: "exited",
  problem: "",
  exited: { code: 255, signal: "", at: "2026-08-26T01:05:00Z" },
  forwards: [],
};

test("reconnects an exited SSH terminal in the same view", async ({ page, installation }) => {
  let session: Record<string, unknown> = exited;
  let ticket = 0;
  let reconnects = 0;

  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/reconnect")) {
      reconnects += 1;
      session = { ...exited, state: "connected", startedAt: "2026-08-26T01:06:00Z" };
      delete session.exited;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ sessions: [session], maxSessions: 12 }),
      });
      return;
    }
    if (path.endsWith("/stream")) {
      ticket += 1;
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ streamTicket: `manual-reconnect-${ticket}` }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ sessions: [session], maxSessions: 12 }),
    });
  });

  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    const current = new URL(socket.url()).searchParams.get("ticket");
    if (current === "manual-reconnect-1") {
      socket.send(Buffer.from("before disconnect\r\n"));
      socket.send(JSON.stringify({ exit: { code: 255, signal: "" } }));
      return;
    }
    socket.send(Buffer.from("[sshc] reconnected; this is a new shell\r\n$ "));
  });

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const terminal = page.getByRole("region", { name: "Terminal for production-api" });
  await expect(terminal).toContainText("before disconnect");
  const reconnect = terminal.getByRole("region", { name: "The program exited with status 255." }).getByRole("button", { name: "Reconnect", exact: true });
  await expect(reconnect).toBeVisible();

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.13.1-terminal-reconnect-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 360, height: 800 });
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.13.1-terminal-reconnect-mobile.png`, fullPage: true });
  }

  await reconnect.click();
  await expect.poll(() => reconnects).toBe(1);
  await expect(terminal).toContainText("reconnected; this is a new shell");
  await expect(reconnect).toBeHidden();
});

// The engine writes this line when a session ends for a reason of its connection (internal/terminal/reconnect.go).
const enterToReconnect = "[sshc] Enterで再接続します。\r\n";

test("after giving up, Enter reconnects and the other keys are dropped", async ({ page, installation }) => {
  const gaveUp = { ...exited, problem: "reconnect_exhausted", exited: { code: -1, signal: "", at: "2026-08-26T01:05:00Z", connectionLost: true } };
  let session: Record<string, unknown> = gaveUp;
  let ticket = 0;
  let reconnects = 0;
  const sentAfterReconnecting: string[] = [];

  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/reconnect")) {
      reconnects += 1;
      session = { ...exited, state: "connected", startedAt: "2026-08-26T01:06:00Z" };
      delete session.exited;
    }
    if (path.endsWith("/stream")) {
      ticket += 1;
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: `gave-up-${ticket}` }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    if (new URL(socket.url()).searchParams.get("ticket") === "gave-up-1") {
      socket.send(Buffer.from(`[sshc] gave up\r\n${enterToReconnect}`));
      socket.send(JSON.stringify({ exit: { code: -1, signal: "", connectionLost: true } }));
      return;
    }
    // What is typed comes as binary frames; the control messages are JSON text.
    socket.onMessage((message) => { if (typeof message !== "string") sentAfterReconnecting.push(message.toString("utf8")); });
    socket.send(Buffer.from("[sshc] reconnected\r\n$ "));
  });

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const terminal = page.getByRole("region", { name: "Terminal for production-api" });
  await expect(terminal).toContainText("Enterで再接続します");
  const card = terminal.getByRole("region", { name: "The SSH connection was lost and did not come back within the reconnect attempts." });
  await expect(card).toBeFocused();
  await card.getByRole("button", { name: "Hide this notice" }).click();

  // With the card hidden the terminal takes the keys, and only Enter does anything.
  await page.keyboard.type("ls");
  expect(reconnects).toBe(0);
  await page.keyboard.press("Enter");
  await expect.poll(() => reconnects).toBe(1);
  await expect(terminal).toContainText("[sshc] reconnected");

  await page.keyboard.type("x");
  await expect.poll(() => sentAfterReconnecting.join("")).toBe("x");
});

test("Enter on the card that took the focus reconnects", async ({ page, installation }) => {
  const gaveUp = { ...exited, problem: "connect_refused" };
  let reconnects = 0;
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/reconnect")) reconnects += 1;
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "refused" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [gaveUp], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(JSON.stringify({ exit: { code: 255, signal: "" } }));
  });

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const card = page.getByRole("region", { name: /^The connection was refused\./ });
  await expect(card).toBeFocused();
  await page.keyboard.press("Enter");
  await expect.poll(() => reconnects).toBe(1);
});

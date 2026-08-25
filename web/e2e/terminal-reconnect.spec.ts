import { expect, openApplication, openSection, test } from "./support/environment";

const exited = {
  id: "manual-reconnect-fixture",
  kind: "ssh",
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
  const console = page.getByRole("region", { name: "Console for production-api" });
  await expect(console).toContainText("before disconnect");
  const reconnect = console.getByRole("button", { name: "Reconnect", exact: true });
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
  await expect(console).toContainText("reconnected; this is a new shell");
  await expect(reconnect).toBeHidden();
});

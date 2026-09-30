import type { WebSocketRoute } from "@playwright/test";
import { expect, openApplication, openSection, test } from "./support/environment";

// The engine only enters the automatic reconnect loop after an established
// SSH connection drops, and the e2e suite has no SSH server to establish one.
// The stop test therefore stands in for the engine with the session list and
// the stream it would send; the engine's side of stopping is covered by
// TestStoppingTheReconnectWaitLeavesAnExitedPaneToReconnectByHand.
const reconnecting = {
  id: "reconnect-stop-fixture",
  kind: "ssh",
  alias: "gateway",
  title: "gateway",
  startedAt: "2026-08-26T01:00:00Z",
  state: "reconnecting",
  problem: "",
  reconnect: { attempt: 3, limit: 5, retryAt: "2026-08-26T01:05:06Z", problem: "reconnect_failed" },
  forwards: [],
};

// JSON.stringify leaves out the undefined reconnect, as the engine does once
// the session has exited. Stopping the wait finishes the session with the exit
// info of the dropped connection, whose code is unknown (terminal.ExitCodeUnknown).
const stopped = {
  ...reconnecting,
  state: "exited",
  problem: "reconnect_stopped",
  reconnect: undefined,
  exited: { code: -1, signal: "", at: "2026-08-26T01:05:02Z" },
};

test("stops the automatic reconnect loop from the pane", async ({ page, installation }) => {
  let session: Record<string, unknown> = reconnecting;
  let stream: WebSocketRoute | undefined;
  let stopRequests = 0;

  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/reconnect/stop")) {
      stopRequests += 1;
      session = stopped;
      // The engine announces the stop on the stream and then ends the program.
      stream?.send(Buffer.from("\r\n[sshc] 再接続を停止しました。\r\n"));
      stream?.send(JSON.stringify({ exit: { code: -1, signal: "" } }));
    }
    if (path.endsWith("/stream")) {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ streamTicket: "reconnect-stop" }),
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
    stream = socket;
    socket.send(Buffer.from("\r\n[sshc] SSH接続が切れました。5秒後に再接続します（3/5）。\r\n"));
  });
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen.getByText("reconnecting 3/5")).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/reconnect-stop-banner.png`, fullPage: true });
  }
  await page.getByRole("button", { name: "Stop reconnecting" }).click();

  await expect.poll(() => stopRequests).toBe(1);
  await expect(screen.getByText("Automatic reconnection was stopped. Reconnect when you are ready.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Reconnect", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop reconnecting" })).toHaveCount(0);
  await expect(screen).toContainText("再接続を停止しました");
  const list = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" });
  await expect(list.getByRole("listitem")).toHaveCount(1);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/reconnect-stopped.png`, fullPage: true });
  }
});

test("does not start the automatic reconnect loop when the first connection fails", async ({ page, installation }) => {
  await installation.write(
    "conf.d/20-refused.conf",
    ["Host refused", "\tHostName 127.0.0.1", "\tPort 1", "\tConnectTimeout 2", ""].join("\n"),
  );
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "refused" }).click();
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  const screen = page.getByRole("region", { name: /^Terminal for / });
  // A loop would keep the pane reconnecting for about 40 seconds before it
  // offered a manual reconnect.
  await expect(screen.getByRole("button", { name: "Reconnect", exact: true })).toBeVisible({ timeout: 20_000 });
  await expect(screen.getByText(/^The connection could not be established\./)).toBeVisible();
  await expect(screen.getByText(/^reconnecting \d/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Stop reconnecting" })).toHaveCount(0);
});

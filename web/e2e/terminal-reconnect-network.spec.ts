import { expect, openApplication, openSection, test } from "./support/environment";

// The engine's side (waiting without using up an attempt, the hint after a new shell) is covered by
// internal/terminal/reconnect_network_test.go and resume_program_test.go. The session list and the stream stand in here.
const session = {
  id: "reconnect-network-fixture",
  kind: "ssh", workspaceId: "default",
  alias: "gateway",
  title: "gateway",
  startedAt: "2026-11-04T01:00:00Z",
  state: "connected",
  problem: "",
  forwards: [],
};

async function playEngine(page: import("@playwright/test").Page, shown: Record<string, unknown>) {
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "network" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [shown], maxSessions: 12 }) });
  });
  const typed: string[] = [];
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.onMessage((message) => { if (typeof message !== "string") typed.push(message.toString("utf8")); });
    socket.send(Buffer.from("$ "));
  });
  return typed;
}

test("says it waits for the network, in the strip, the card and the row", async ({ page, installation }) => {
  await playEngine(page, {
    ...session,
    state: "reconnecting",
    reconnect: { attempt: 2, limit: 5, retryAt: "2026-11-04T01:00:00Z", problem: "connect_no_network", waitingForNetwork: true },
  });
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const screen = page.getByRole("region", { name: "Terminal for gateway" });
  await expect(screen.getByText("Waiting for the network. Reconnects as soon as it is back.")).toBeVisible();
  const card = screen.getByRole("region", { name: "The connection to gateway was lost" });
  await expect(card).toContainText("Reconnects as soon as the network is back.");
  await expect(card).not.toContainText("2/5");
  await expect(page.locator("[data-session-status-text]")).toHaveText("Waiting for the network");
});

test("offers to continue the previous shell's program without taking the focus from the terminal", async ({ page, installation }) => {
  const typed = await playEngine(page, { ...session, resume: { program: "Claude Code", command: "claude --continue" } });
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const screen = page.getByRole("region", { name: "Terminal for gateway" });
  const card = screen.getByRole("region", { name: "Claude Code was running in the previous shell" });
  await expect(card).toContainText("claude --continue");
  await expect(card).not.toBeFocused();

  await card.getByRole("button", { name: "Continue where it left off" }).click();
  await expect.poll(() => typed.join("")).toBe("claude --continue\r");
});

import { expect, openApplication, openSection, test } from "./support/environment";

const connecting = {
  id: "proxy-jump-progress",
  kind: "ssh",
  alias: "jamstec-moon07-mdx01-tunnel",
  title: "jamstec-moon07-mdx01-tunnel",
  startedAt: "2026-08-28T00:00:00Z",
  state: "connecting",
  problem: "",
  progress: {
    phase: "authenticating",
    alias: "mdx-jamstec-1",
    hostName: "192.0.2.10",
    user: "ops",
    hop: 1,
    hops: 2,
  },
  forwards: [],
};

test("shows the current ProxyJump hop while SSH is authenticating", async ({ page, installation }) => {
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ streamTicket: "proxy-jump-progress" }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ sessions: [connecting], maxSessions: 12 }),
    });
  });
  await page.routeWebSocket("**/terminal/stream?**", () => {});

  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const status = "authenticating with mdx-jamstec-1 · 1/2";
  await expect(page.getByText(status)).toHaveCount(2);

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-terminal-progress-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 360, height: 800 });
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${visualDirectory}/sshc-terminal-progress-mobile.png`, fullPage: true });
  }
});

import { expect, openApplication, openSection, test } from "./support/environment";

// A connection that keeps failing enters the automatic reconnect loop with a
// growing backoff. The user can stop that loop from the pane and the session
// stays listed, exited, with a manual reconnect still available.
test("stops the automatic reconnect loop from the pane", async ({ page, installation }) => {
  await installation.write(
    "conf.d/20-refused.conf",
    ["Host refused", "\tHostName 127.0.0.1", "\tPort 1", "\tConnectTimeout 2", ""].join("\n"),
  );
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "refused" }).click();
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  // The third wait is six seconds long, enough to act on the banner.
  await expect(screen.getByText("reconnecting 3/5")).toBeVisible({ timeout: 30_000 });
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/reconnect-stop-banner.png`, fullPage: true });
  }
  await page.getByRole("button", { name: "Stop reconnecting" }).click();

  await expect(screen.getByText("Automatic reconnection was stopped. Reconnect when you are ready.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Reconnect", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Stop reconnecting" })).toHaveCount(0);
  await expect(screen).toContainText("再接続を停止しました");
  const list = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open consoles" });
  await expect(list.getByRole("listitem")).toHaveCount(1);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/reconnect-stopped.png`, fullPage: true });
  }
});

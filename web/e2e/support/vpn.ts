import type { Page } from "@playwright/test";

// The VPN overview (GET /api/v1/vpn). The screen reads it with and without
// waitForRoutes, so the query is optional.
export const vpnOverviewPath = /\/api\/v1\/vpn(\?.*)?$/;

// Opening the VPN screen makes the engine ask Docker for its containers, so
// what the screen shows would depend on the machine running the tests. Tests
// that only need the screen to open answer the overview with no profiles.
export async function answerNoVPNProfiles(page: Page): Promise<void> {
  await page.route(vpnOverviewPath, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ available: true, checking: false, profiles: [] }),
    }));
}

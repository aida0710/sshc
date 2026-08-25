import { expect, openApplication, openSection, test } from "./support/environment";

test("keeps another page working when one page renews its CSRF token", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  // Simulate another page renewing the same cookie while this page still holds the
  // previous token. Deliberately discard the newly returned tab token.
  expect(await page.evaluate(async () => {
    const csrf = window.sessionStorage.getItem("sshc.session.csrf") ?? "";
    const response = await fetch("/api/v1/session/renew", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-SSHC-CSRF": csrf },
    });
    return response.ok;
  })).toBe(true);

  await openSection(page, "History");

  await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();
});

test("replaces the entire shell with a centered recovery screen when its session is gone", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await page.route("**/api/v1/history", async (route) => {
    await route.fulfill({
      status: 401,
      contentType: "application/problem+json",
      body: JSON.stringify({ code: "invalid_session", message: "request rejected" }),
    });
  });

  await openSection(page, "History");

  await expect(page.getByRole("heading", { name: "Session ended" })).toBeVisible();
  await expect(page.getByRole("alert")).toContainText("Reload to renew the local session");
  await expect(page.getByRole("navigation", { name: "Primary" })).toHaveCount(0);
  const recovery = page.getByRole("heading", { name: "Session ended" }).locator("..");
  const box = await recovery.boundingBox();
  expect(box).not.toBeNull();
  if (box !== null) {
    expect(Math.abs(box.x + box.width / 2 - 640)).toBeLessThan(2);
    expect(Math.abs(box.y + box.height / 2 - 360)).toBeLessThan(2);
  }
});

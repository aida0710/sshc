import { expect, openApplication, openSection, test } from "./support/environment";

function atPath(url: string, pathname: string): string {
  const source = new URL(url);
  const destination = new URL(pathname, source.origin);
  destination.hash = source.hash;
  return destination.toString();
}

test("opens, reloads, and traverses section URLs", async ({ page, installation }) => {
  await openApplication(page, { url: atPath(installation.url, "/connections") });

  await expect(page).toHaveURL(/\/connections$/);
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();

  await openSection(page, "Keys");
  await expect(page).toHaveURL(/\/keys$/);
  await expect(page.getByRole("heading", { name: "Keys", level: 2 })).toBeVisible();

  await openSection(page, "History");
  await expect(page).toHaveURL(/\/history$/);
  await page.goBack();
  await expect(page).toHaveURL(/\/keys$/);
  await expect(page.getByRole("heading", { name: "Keys", level: 2 })).toBeVisible();

  await page.goForward();
  await expect(page).toHaveURL(/\/history$/);
  await expect(page.getByRole("heading", { name: "History", level: 2 })).toBeVisible();

  await page.reload();
  await expect(page).toHaveURL(/\/history$/);
  await expect(page.getByRole("heading", { name: "History", level: 2 })).toBeVisible();
});

test("normalizes one trailing slash without leaving the requested section", async ({
  page,
  installation,
}) => {
  await openApplication(page, { url: atPath(installation.url, "/connections/") });

  expect(new URL(page.url()).pathname).toBe("/connections");
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Connections", exact: true })).toHaveAttribute(
    "aria-current",
    "page",
  );
});

test("keeps an unknown URL until the person chooses Home", async ({ page, installation }) => {
  await openApplication(page, { url: atPath(installation.url, "/missing") });

  await expect(page).toHaveURL(/\/missing$/);
  await expect(page.getByRole("heading", { name: "Page not found", level: 2 })).toBeVisible();
  await expect(page.getByText("/missing", { exact: true })).toBeVisible();

  await page.getByRole("link", { name: "Go to Home" }).click();
  expect(new URL(page.url()).pathname).toBe("/");
  await expect(page.getByRole("heading", { name: "Your connections", level: 2 })).toBeVisible();
});

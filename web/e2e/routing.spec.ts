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

  await openSection(page, "Settings");
  await expect(page).toHaveURL(/\/settings$/);
  await expect(page.getByRole("heading", { name: "Settings", level: 2 })).toBeVisible();

  await openSection(page, "History");
  await expect(page).toHaveURL(/\/history$/);
  await page.goBack();
  await expect(page).toHaveURL(/\/settings$/);
  await expect(page.getByRole("heading", { name: "Settings", level: 2 })).toBeVisible();

  await page.goForward();
  await expect(page).toHaveURL(/\/history$/);
  await expect(page.getByRole("heading", { name: "History", level: 2 })).toBeVisible();

  await page.reload();
  await expect(page).toHaveURL(/\/history$/);
  await expect(page.getByRole("heading", { name: "History", level: 2 })).toBeVisible();
});

test("restores a selected connection and editor tab from its URL", async ({
  page,
  installation,
}) => {
  await openApplication(page, { url: atPath(installation.url, "/connections") });

  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
    .click();
  let current = new URL(page.url());
  expect(current.pathname).toBe("/connections");
  expect(Object.fromEntries(current.searchParams)).toEqual({
    path: "config",
    host: "bastion",
    tab: "basic",
  });

  await page.getByRole("tab", { name: "Advanced settings" }).click();
  await page.getByRole("tab", { name: "Directives" }).click();
  current = new URL(page.url());
  expect(current.searchParams.get("tab")).toBe("advanced");

  await page.reload();
  await expect(page.getByRole("heading", { name: "bastion", exact: true })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Advanced settings" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByRole("tab", { name: "Directives" })).toHaveAttribute("aria-selected", "true");

  await page.goBack();
  await expect(page.getByRole("tab", { name: "Jump" })).toHaveAttribute("aria-selected", "true");
  expect(new URL(page.url()).searchParams.get("tab")).toBe("jump");
  await page.goBack();
  await expect(page.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
  expect(new URL(page.url()).searchParams.get("tab")).toBe("basic");
});

test("maps every legacy connection tab URL without starting an operation", async ({
  page,
  installation,
}) => {
  const started: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (request.method() === "POST" &&
      (path.startsWith("/api/v1/diagnostics/") || path === "/api/v1/terminal/launch")) {
      started.push(path);
    }
  });
  await openApplication(page, { url: atPath(installation.url, "/connections") });

  const cases = [
    { tab: "basic", area: "Basic" },
    { tab: "diagnostics", area: "Basic", region: "Connection checks" },
    { tab: "effective", area: "Settings analysis" },
    { tab: "jump", area: "Advanced settings", advanced: "Jump" },
    { tab: "advanced", area: "Advanced settings", advanced: "Directives" },
    { tab: "raw", area: "Advanced settings", advanced: "Raw" },
  ];

  for (const item of cases) {
    const target = new URL("/connections", page.url());
    target.searchParams.set("path", "config");
    target.searchParams.set("host", "bastion");
    target.searchParams.set("tab", item.tab);
    await page.goto(target.toString());
    await expect(page.getByRole("tab", { name: item.area })).toHaveAttribute("aria-selected", "true");
    if (item.advanced !== undefined) {
      await expect(page.getByRole("tab", { name: item.advanced })).toHaveAttribute("aria-selected", "true");
    }
    if (item.region !== undefined) {
      await expect(page.getByRole("region", { name: item.region })).toBeVisible();
    }
  }

  expect(started).toEqual([]);
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

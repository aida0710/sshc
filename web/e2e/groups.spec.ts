import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";

test("declares a group in the entry file and moves a connection into it", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");

  await page.getByLabel("New group name").fill("work");
  await page.getByRole("button", { name: "Add group" }).click();
  await expect(page.getByRole("region", { name: "Unsaved group changes" })).toHaveCSS("position", "static");
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  const entry = await installation.read("config");
  expect(entry).toContain("# >>> sshc groups (generated).");
  expect(entry).toContain("Include connections/work/*.conf\n");
  expect(entry).toContain("Include groups.sshc.conf\n");
  expect(entry).toContain("# <<< sshc groups");
  expect(entry.indexOf("# >>> sshc groups")).toBeLessThan(entry.indexOf("Host "));

  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas" })
    .click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();

  await page.getByRole("button", { name: "More connection actions" }).click();
  await page.getByLabel("Primary group").selectOption("work");
  expect(await clickAndAwait(page, "Move to this group", "/api/v1/config/save")).toBe(200);

  expect(await installation.read("connections/work/10-home.conf")).toContain("Host nas");
  expect(await installation.read("conf.d/10-home.conf")).toBe("");
});

test("gives a nested group its own Include line, deepest first", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");

  for (const name of ["work", "work/eu"]) {
    await page.getByLabel("New group name").fill(name);
    await page.getByRole("button", { name: "Add group" }).click();
  }
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  const entry = await installation.read("config");
  const child = entry.indexOf("Include connections/work/eu/*.conf");
  const parent = entry.indexOf("Include connections/work/*.conf");
  expect(child).toBeGreaterThan(-1);
  expect(parent).toBeGreaterThan(-1);
  expect(child).toBeLessThan(parent);
});

test("renames a group and carries its files, its Include line and its keys", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");

  await page.getByLabel("New group name").fill("work");
  await page.getByRole("button", { name: "Add group" }).click();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas" })
    .click();
  await page.getByRole("button", { name: "More connection actions" }).click();
  await page.getByLabel("Primary group").selectOption("work");
  expect(await clickAndAwait(page, "Move to this group", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Groups");
  await page.getByRole("heading", { name: "work", exact: true }).click();
  await page.getByLabel("Rename work to").fill("client-a");
  expect(await clickAndAwait(page, "Rename work", "/api/v1/config/groups/rename")).toBe(200);

  expect(await installation.read("connections/client-a/10-home.conf")).toContain("Host nas");
  const entry = await installation.read("config");
  expect(entry).toContain("Include connections/client-a/*.conf\n");
  expect(entry).not.toContain("Include connections/work/*.conf\n");
});

test("refuses to move a connection into a group nothing declares", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas" })
    .click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();

  await page.getByRole("button", { name: "More connection actions" }).click();
  await expect(page.getByRole("button", { name: "Move to this group" })).toBeDisabled();
  expect(await installation.read("conf.d/10-home.conf")).toContain("Host nas");
});

test("quick connect drills into a nested group and promotes it when its container is hidden", async ({
  page,
  installation,
}) => {
  const terminalRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/v1/terminal/")) return;
    if (request.method() === "GET") return;
    terminalRequests.push(`${request.method()} ${path}`);
  });
  await openApplication(page, installation);
  await openSection(page, "Groups");
  for (const name of ["work", "work/eu"]) {
    await page.getByLabel("New group name").fill(name);
    await page.getByRole("button", { name: "Add group" }).click();
  }
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Home");
  let browser = page.getByRole("region", { name: "Quick connect" });
  expect(new URL(page.url()).pathname).toBe("/");

  await expect(browser.getByRole("button", { name: "Open work, 0 connections" })).toBeVisible();
  await expect(browser.getByRole("button", { name: "Open work/eu, 0 connections" })).toHaveCount(0);
  await browser.getByRole("button", { name: "Open work, 0 connections" }).click();
  expect(new URL(page.url()).pathname).toBe("/");
  await expect(browser.getByRole("button", { name: "Open work/eu, 0 connections" })).toBeVisible();
  await browser.getByRole("button", { name: "Open work/eu, 0 connections" }).click();
  expect(new URL(page.url()).pathname).toBe("/");
  await expect(
    browser.getByRole("navigation", { name: "Selected group" }).getByText("eu", { exact: true }),
  ).toHaveAttribute("aria-current", "page");
  expect(terminalRequests).toEqual([]);

  await page.reload();
  browser = page.getByRole("region", { name: "Quick connect" });
  await expect(browser.getByRole("button", { name: "Open work, 0 connections" })).toBeVisible();

  await openSection(page, "Groups");
  await page.getByRole("listitem").filter({ hasText: "work" }).first().click();
  await page.getByRole("button", { name: "Show Group display settings" }).click();
  const hide = page.getByLabel("Hide work from Connections");
  await hide.click();
  await expect(hide).toBeChecked();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Home");
  browser = page.getByRole("region", { name: "Quick connect" });
  await expect(browser.getByRole("button", { name: "Open work, 0 connections" })).toHaveCount(0);
  const promoted = browser.getByRole("button", { name: "Open work/eu, 0 connections" });
  await expect(promoted).toBeVisible();
  await promoted.click();
  expect(new URL(page.url()).pathname).toBe("/");
});

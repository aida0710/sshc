import { chooseConnectionAction } from "./support/connections";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";
import { createGroups, groupList, openGroupMenu } from "./support/groups";

// moveNasInto moves the fixture's nas connection into a group from the 「…」 in its header.
async function moveNasInto(page: Page, group: string) {
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "nas", exact: true }).click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();
  await chooseConnectionAction(page, "Move to group…");
  await page.getByLabel("Primary group").selectOption(group);
  expect(await clickAndAwait(page, "Move to this group", "/api/v1/config/save")).toBe(200);
}

async function editGroup(page: Page, name: string) {
  await openGroupMenu(page, name);
  await page.getByRole("menuitem", { name: "Edit" }).click();
  await expect(page.locator("#group-editor-heading")).toHaveText(name);
}

test("declares a group in the entry file and moves a connection into it", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["work"]);

  const entry = await installation.read("config");
  expect(entry).toContain("# >>> sshc groups (generated).");
  expect(entry).toContain("Include connections/work/*.conf\n");
  expect(entry).toContain("Include groups.sshc.conf\n");
  expect(entry).toContain("# <<< sshc groups");
  expect(entry.indexOf("# >>> sshc groups")).toBeLessThan(entry.indexOf("Host "));

  await moveNasInto(page, "work");
  expect(await installation.read("connections/work/10-home.conf")).toContain("Host nas");
  expect(await installation.read("conf.d/10-home.conf")).toBe("");
});

test("lists a group member once after the group's settings are saved", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["work"]);
  await moveNasInto(page, "work");

  await editGroup(page, "work");
  await page.getByText("Other settings", { exact: true }).click();
  await page.getByLabel("Setting", { exact: true }).fill("ServerAliveInterval");
  await page.getByLabel("Value", { exact: true }).fill("15");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  expect(await clickAndAwait(page, "Save group", "/api/v1/config/save")).toBe(200);
  // groups.sshc.conf の Host 行は nas の alias を宣言するが、接続先ではない。
  expect(await installation.read("groups.sshc.conf")).toContain("Host nas\n\tServerAliveInterval 15\n");

  await groupList(page).getByRole("button", { name: "All", exact: true }).click();
  const nas = page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "nas", exact: true });
  await expect(nas).toHaveCount(1);
  await expect(nas).not.toHaveAccessibleDescription(/duplicate alias/);
  await nas.click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();
  await expect(page.getByText("Another block declares the same alias.")).toHaveCount(0);
});

test("gives a nested group its own Include line, deepest first", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["work", "work/eu"]);

  const entry = await installation.read("config");
  const child = entry.indexOf("Include connections/work/eu/*.conf");
  const parent = entry.indexOf("Include connections/work/*.conf");
  expect(child).toBeGreaterThan(-1);
  expect(parent).toBeGreaterThan(-1);
  expect(child).toBeLessThan(parent);
});

test("renames a group and carries its files and its Include line", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["work"]);
  await moveNasInto(page, "work");

  await openGroupMenu(page, "work");
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const dialog = page.getByRole("dialog", { name: "Rename work" });
  await dialog.getByLabel("Name", { exact: true }).fill("client-a");
  const [response] = await Promise.all([
    page.waitForResponse((candidate) => candidate.url().includes("/api/v1/config/groups/rename")),
    dialog.getByRole("button", { name: "Rename", exact: true }).click(),
  ]);
  expect(response.status()).toBe(200);

  expect(await installation.read("connections/client-a/10-home.conf")).toContain("Host nas");
  const entry = await installation.read("config");
  expect(entry).toContain("Include connections/client-a/*.conf\n");
  expect(entry).not.toContain("Include connections/work/*.conf\n");
});

test("refuses to move a connection into a group nothing declares", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "nas", exact: true }).click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();

  await chooseConnectionAction(page, "Move to group…");
  await expect(page.getByRole("button", { name: "Move to this group" })).toBeDisabled();
  expect(await installation.read("conf.d/10-home.conf")).toContain("Host nas");
});

test("shows the files and keys in a group's folders", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["lab"]);
  await installation.write("connections/lab/10-gpu.conf", "Host gpu-01\n\tHostName 192.0.2.31\n\nHost gpu-02\n\tHostName 192.0.2.32\n");

  await openSection(page, "Keys");
  await page.getByLabel("File name").fill("id_lab");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);
  await page.getByRole("checkbox", { name: "Choose id_lab" }).check();
  await page.getByRole("combobox", { name: "Move into" }).selectOption("lab");
  expect(await clickAndAwait(page, "Move", "/api/v1/keys/")).toBe(200);

  await openSection(page, "Connections");
  await editGroup(page, "lab");
  const folders = page.getByRole("region", { name: "Folders" });
  await expect(folders.getByText("connections/lab/", { exact: true })).toBeVisible();
  await expect(folders.getByText("10-gpu.conf", { exact: true })).toBeVisible();
  await expect(folders.getByText("gpu-01, gpu-02", { exact: true })).toBeVisible();
  await expect(folders.getByText("keys/lab/", { exact: true })).toBeVisible();
  await expect(folders.getByText("id_lab", { exact: true })).toBeVisible();
  await expect(folders.getByText(".pub", { exact: true })).toBeVisible();
  await expect(folders.getByText("Include connections/lab/*.conf", { exact: true })).toBeVisible();
});

test("moves a group up and down among its siblings", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["alpha", "beta", "gamma"]);
  const order = () => groupList(page).getByRole("button", { name: /^(alpha|beta|gamma)$/ }).allTextContents();
  await expect.poll(async () => (await order()).map((text) => text.replace(/\d+$/, ""))).toEqual(["alpha", "beta", "gamma"]);

  await openGroupMenu(page, "gamma");
  await page.getByRole("menuitem", { name: "Move up" }).click();
  await expect.poll(async () => (await order()).map((text) => text.replace(/\d+$/, ""))).toEqual(["alpha", "gamma", "beta"]);

  await openGroupMenu(page, "alpha");
  await expect(page.getByRole("menuitem", { name: "Move up" })).toBeDisabled();
  await page.getByRole("menuitem", { name: "Move down" }).click();
  await expect.poll(async () => (await order()).map((text) => text.replace(/\d+$/, ""))).toEqual(["gamma", "alpha", "beta"]);
});

test("hides only a group with no connections of its own and brings it back on request", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["work", "shelf"]);
  await moveNasInto(page, "work");

  await openGroupMenu(page, "work");
  await expect(page.getByRole("menuitem", { name: "Hide in Connections" })).toBeDisabled();
  await page.keyboard.press("Escape");

  await openGroupMenu(page, "shelf");
  const [response] = await Promise.all([
    page.waitForResponse((candidate) => candidate.url().includes("/api/v1/config/save") && candidate.request().method() === "POST"),
    page.getByRole("menuitem", { name: "Hide in Connections" }).click(),
  ]);
  expect(response.status()).toBe(200);
  await expect(groupList(page).getByRole("button", { name: "shelf", exact: true })).toHaveCount(0);

  await groupList(page).getByRole("button", { name: "Show hidden groups (1)" }).click();
  await expect(groupList(page).getByRole("button", { name: "shelf", exact: true })).toContainText("Hidden");
  await openGroupMenu(page, "shelf");
  await page.getByRole("menuitem", { name: "Stop hiding" }).click();
  await expect(groupList(page).getByRole("button", { name: "Show hidden groups (1)" })).toHaveCount(0);
  await expect(groupList(page).getByRole("button", { name: "shelf", exact: true })).not.toContainText("Hidden");
});

test("reports a folder that no Include line reads and makes it a group", async ({ page, installation }) => {
  await mkdir(join(installation.home, ".ssh", "connections", "old"), { recursive: true });
  await installation.write("connections/old/10-old.conf", "Host legacy\n\tHostName 192.0.2.99\n");
  await openApplication(page, installation);
  await openSection(page, "Connections");

  const checks = groupList(page).getByText("Folder checks: 1", { exact: true });
  await checks.click();
  await expect(groupList(page).getByText("connections/old/ is not read: no Include line names it.")).toBeVisible();
  expect(await clickAndAwait(page, "Make it a group", "/api/v1/config/save")).toBe(200);

  await expect(checks).toHaveCount(0);
  await expect(groupList(page).getByRole("button", { name: "old", exact: true })).toBeVisible();
  expect(await installation.read("config")).toContain("Include connections/old/*.conf\n");
  await expect(page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "legacy", exact: true })).toBeVisible();
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
  await createGroups(page, ["work", "work/eu"]);

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

  await openSection(page, "Connections");
  await openGroupMenu(page, "work");
  const [hidden] = await Promise.all([
    page.waitForResponse((candidate) => candidate.url().includes("/api/v1/config/save") && candidate.request().method() === "POST"),
    page.getByRole("menuitem", { name: "Hide in Connections" }).click(),
  ]);
  expect(hidden.status()).toBe(200);

  await openSection(page, "Home");
  browser = page.getByRole("region", { name: "Quick connect" });
  await expect(browser.getByRole("button", { name: "Open work, 0 connections" })).toHaveCount(0);
  const promoted = browser.getByRole("button", { name: "Open work/eu, 0 connections" });
  await expect(promoted).toBeVisible();
  await promoted.click();
  expect(new URL(page.url()).pathname).toBe("/");
});

import { chooseConnectionAction } from "./support/connections";
import type { Page } from "@playwright/test";
import { expect, openApplication, openSection, test, type Installation } from "./support/environment";

// The entry file keeps its banner and declares one group, "work", with one member.
// alpha and beta start in the entry file, so they share one file and one save precondition.
async function installWorkGroup(installation: Installation) {
  const entry = await installation.read("config");
  await installation.write(
    "config",
    "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.\n" +
      "# Edit through the UI; lines between these markers are replaced on the next save.\n" +
      "Include connections/work/*.conf\n" +
      "Include groups.sshc.conf\n" +
      "# <<< sshc groups\n" +
      entry +
      "\nHost alpha\n\tHostName 192.0.2.11\n\nHost beta\n\tHostName 192.0.2.12\n",
  );
  await installation.write("connections/work/web.conf", "Host web\n\tHostName 192.0.2.20\n");
}

async function openConnections(page: Page, installation: Installation) {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  return page.getByRole("navigation", { name: "Connections" });
}

// The list of groups sits beside the Connections card, apart from the connection list.
function groupList(page: Page) {
  return page.getByRole("navigation", { name: "Groups" });
}

function savesTo(page: Page) {
  const requests: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().includes("/api/v1/config/save")) requests.push(request.postData() ?? "");
  });
  return requests;
}

test("moves two connections out of the shared entry file into a group with one save", async ({ page, installation }) => {
  await installWorkGroup(installation);
  const tree = await openConnections(page, installation);
  const saves = savesTo(page);

  await tree.getByRole("button", { name: "alpha", exact: true }).click();
  await tree.getByRole("button", { name: "beta", exact: true }).click({ modifiers: ["Control"] });
  const bar = page.getByRole("toolbar", { name: "2 selected" });
  await bar.getByRole("button", { name: "Move to group…" }).click();
  await page.getByRole("dialog", { name: "Move 2 connections" }).getByRole("button", { name: "work", exact: true }).click();

  const work = tree.getByRole("region", { name: "work group, 3 connections" });
  await expect(work.getByRole("button", { name: "alpha", exact: true })).toBeVisible();
  await expect(work.getByRole("button", { name: "beta", exact: true })).toBeVisible();
  expect(saves).toHaveLength(1);
  expect(JSON.parse(saves[0] ?? "{}")).toMatchObject({ kind: "move_hosts", destinationGroup: "work" });
  const entry = await installation.read("config");
  expect(entry).not.toContain("Host alpha");
  expect(entry).not.toContain("Host beta");
  expect(entry).toContain("# Managed by hand since 2019. Do not reformat.");
});

test("deletes the chosen connections together after confirming", async ({ page, installation }) => {
  await installWorkGroup(installation);
  const tree = await openConnections(page, installation);

  // Shift chooses alpha, bastion and beta in name order; Ctrl takes bastion back out.
  await tree.getByRole("button", { name: "alpha", exact: true }).click();
  await tree.getByRole("button", { name: "beta", exact: true }).click({ modifiers: ["Shift"] });
  await expect(page.getByRole("toolbar", { name: "3 selected" })).toBeVisible();
  await tree.getByRole("button", { name: "bastion", exact: true }).click({ modifiers: ["Control"] });
  await page.getByRole("toolbar", { name: "2 selected" }).getByRole("button", { name: "Delete…" }).click();
  const confirm = page.getByRole("dialog", { name: "Delete 2 connections?" });
  await expect(confirm).toContainText("This removes these Host blocks from your configuration.");
  await confirm.getByRole("button", { name: "Delete 2" }).click();

  await expect(tree.getByRole("button", { name: "alpha", exact: true })).toHaveCount(0);
  await expect(tree.getByRole("button", { name: "beta", exact: true })).toHaveCount(0);
  const entry = await installation.read("config");
  expect(entry).toContain("Host bastion");
  expect(entry).not.toContain("Host alpha");
});

test("creates, renames and deletes a group from the Connections group column", async ({ page, installation }) => {
  await installWorkGroup(installation);
  await openConnections(page, installation);

  await groupList(page).getByRole("button", { name: "New group" }).click();
  const create = page.getByRole("dialog", { name: "New group" });
  await create.getByLabel("Name").fill("lab");
  await create.getByRole("button", { name: "Create" }).click();
  await expect(groupList(page).getByRole("button", { name: "lab", exact: true })).toBeVisible();
  expect(await installation.read("config")).toContain("Include connections/lab/*.conf");

  await groupList(page).getByRole("button", { name: "lab", exact: true }).hover();
  await groupList(page).getByRole("button", { name: "Actions for lab" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const rename = page.getByRole("dialog", { name: "Rename lab" });
  await rename.getByLabel("Name").fill("homelab");
  await rename.getByRole("button", { name: "Rename" }).click();
  await expect(groupList(page).getByRole("button", { name: "homelab", exact: true })).toBeVisible();

  await groupList(page).getByRole("button", { name: "homelab", exact: true }).hover();
  await groupList(page).getByRole("button", { name: "Actions for homelab" }).click();
  await page.getByRole("menuitem", { name: "Delete…" }).click();
  await page.getByRole("dialog", { name: "Delete homelab?" }).getByRole("button", { name: "Delete group" }).click();
  await expect(groupList(page).getByRole("button", { name: "homelab", exact: true })).toHaveCount(0);
  expect(await installation.read("config")).not.toContain("connections/homelab");
});

test("saves a group's defaults from the editor shown beside its connections", async ({ page, installation }) => {
  await installWorkGroup(installation);
  await openConnections(page, installation);

  await groupList(page).getByRole("button", { name: "work", exact: true }).click();
  const editor = page.getByRole("region", { name: "work", exact: true });
  await editor.getByLabel("User name").fill("deploy");
  await editor.getByLabel("Jump host").fill("bastion");
  await page.getByRole("button", { name: "Save group" }).click();
  await expect(page.getByRole("button", { name: "Save group" })).toHaveCount(0);

  const settings = await installation.read("groups.sshc.conf");
  expect(settings).toContain("# group work");
  await expect(editor.getByLabel("User name")).toHaveValue("deploy");
});

test("asks before dropping a group's unsaved defaults when another group is chosen", async ({ page, installation }) => {
  await installWorkGroup(installation);
  await openConnections(page, installation);

  await groupList(page).getByRole("button", { name: "work", exact: true }).click();
  const editor = page.getByRole("region", { name: "work", exact: true });
  await editor.getByLabel("User name").fill("deploy");
  await groupList(page).getByRole("button", { name: "All", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "Discard changes" });
  await expect(confirm).toContainText("Discard the unsaved group settings?");
  await confirm.getByRole("button", { name: "Keep editing" }).click();
  await expect(editor.getByLabel("User name")).toHaveValue("deploy");

  await groupList(page).getByRole("button", { name: "All", exact: true }).click();
  await page.getByRole("dialog", { name: "Discard changes" }).getByRole("button", { name: "Discard changes" }).click();
  await expect(editor).toHaveCount(0);
  await expect(installation.read("groups.sshc.conf")).rejects.toThrow();
});

test("keeps the open connection open when it was already in the group the others moved to", async ({ page, installation }) => {
  await installWorkGroup(installation);
  const tree = await openConnections(page, installation);

  await tree.getByRole("button", { name: "web", exact: true }).click();
  await expect(page.getByRole("heading", { name: "web", exact: true })).toBeVisible();
  await tree.getByRole("button", { name: "alpha", exact: true }).click({ modifiers: ["Control"] });
  await page.getByRole("toolbar", { name: "2 selected" }).getByRole("button", { name: "Move to group…" }).click();
  await page.getByRole("dialog", { name: "Move 2 connections" }).getByRole("button", { name: "work" }).click();

  await expect(tree.getByRole("region", { name: "work group, 2 connections" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "web", exact: true })).toBeVisible();
});

test("follows the open connection and the group editor when their group is renamed", async ({ page, installation }) => {
  await installWorkGroup(installation);
  const tree = await openConnections(page, installation);

  await tree.getByRole("button", { name: "web", exact: true }).click();
  await groupList(page).getByRole("button", { name: "work", exact: true }).hover();
  await groupList(page).getByRole("button", { name: "Actions for work" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const rename = page.getByRole("dialog", { name: "Rename work" });
  await rename.getByLabel("Name").fill("lab");
  await rename.getByRole("button", { name: "Rename" }).click();
  await expect(groupList(page).getByRole("button", { name: "lab", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "web", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/path=connections%2Flab%2Fweb\.conf/);

  await groupList(page).getByRole("button", { name: "lab", exact: true }).hover();
  await groupList(page).getByRole("button", { name: "Actions for lab" }).click();
  await page.getByRole("menuitem", { name: "Edit" }).click();
  await expect(page.getByRole("region", { name: "lab", exact: true })).toBeVisible();
  await groupList(page).getByRole("button", { name: "lab", exact: true }).hover();
  await groupList(page).getByRole("button", { name: "Actions for lab" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const second = page.getByRole("dialog", { name: "Rename lab" });
  await second.getByLabel("Name").fill("ops");
  await second.getByRole("button", { name: "Rename" }).click();
  await expect(page.getByRole("region", { name: "ops", exact: true })).toBeVisible();
});

test("deletes the open connection from its header menu with the file's contents sent once", async ({ page, installation }) => {
  await installWorkGroup(installation);
  const tree = await openConnections(page, installation);
  const saves = savesTo(page);

  await tree.getByRole("button", { name: "alpha", exact: true }).click();
  await chooseConnectionAction(page, "Delete…");
  await page.getByRole("button", { name: "Delete it" }).click();

  await expect(tree.getByRole("button", { name: "alpha", exact: true })).toHaveCount(0);
  expect(JSON.parse(saves[0] ?? "{}")).toMatchObject({ kind: "delete_hosts", hosts: [{ path: "config", alias: "alpha" }] });
  expect(await installation.read("config")).toContain("Host beta");
});

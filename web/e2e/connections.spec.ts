import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";
import type { Page } from "@playwright/test";

async function openBastion(page: Page, url: string) {
  await openApplication(page, { url });
  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
    .click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();
}

test("separates classification, filtered results, and connection detail without losing management controls", async ({
  page,
  installation,
}) => {
  const entry = await installation.read("config");
  const nas = await installation.read("conf.d/10-home.conf");
  await installation.write(
    "config",
      "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.\n" +
      "# Edit through the UI; lines between these markers are replaced on the next save.\n" +
      "Include connections/home/eu/*.conf\n" +
      "Include connections/home/*.conf\n" +
      "Include connections/work/*.conf\n" +
      "Include groups.sshc.conf\n" +
      "# <<< sshc groups\n" + entry,
  );
  await installation.write("connections/home/nas.conf", nas);
  await installation.write(
    "connections/home/eu/api.conf",
    "Host eu-api\n\tHostName 198.51.100.21\n\tUser aida\n",
  );
  await installation.write("conf.d/10-home.conf", "");
  await openApplication(page, installation);
  await openSection(page, "Connections");
  const browser = page.getByRole("navigation", { name: "Connections" });
  const results = page.getByRole("region", { name: "Connection results" });

  await expect(browser.getByRole("group", { name: "Arrange connections by" })).toHaveCount(0);
  await expect(results.getByText("ops@203.0.113.10:2222", { exact: true })).toBeVisible();
  await expect(results.getByText("aida@198.51.100.20", { exact: true })).toBeVisible();
  await expect(results.getByRole("region", { name: "home group, 1 connections" })).toBeVisible();
  await expect(results.getByRole("region", { name: "home/eu group, 1 connections" })).toBeVisible();
  await expect(results.getByRole("region", { name: "Ungrouped group, 1 connections" })).toBeVisible();

  await browser.getByRole("button", { name: "home", exact: true }).click();
  await expect(results.getByRole("button", { name: "nas" })).toBeVisible();
  await expect(results.getByRole("button", { name: "eu-api" })).toBeVisible();
  await expect(results.getByRole("button", { name: "bastion" })).toHaveCount(0);
  await browser.getByRole("button", { name: "home/eu", exact: true }).click();
  await expect(results.getByRole("region", { name: "home/eu group, 1 connections" })).toBeVisible();
  await expect(results.getByRole("button", { name: "nas" })).toHaveCount(0);

  await browser.getByRole("button", { name: "All", exact: true }).click();
  await browser.getByRole("searchbox", { name: "Filter connections" }).fill("198.51.100.20");
  await expect(results.getByRole("button", { name: "nas" })).toBeVisible();
  await browser.getByRole("searchbox", { name: "Filter connections" }).fill("");
  await results.getByRole("button", { name: "bastion" }).click();
  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();
  await expect(browser).toBeVisible();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-connections-management-desktop.png`,
      fullPage: true,
    });
  }
});

test("keeps the selected connection open when its tree item is clicked again", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);
  const connectionURL = page.url();

  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
    .click();

  await expect(page.getByRole("tablist", { name: "Connection editor" })).toBeVisible();
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2222");
  expect(page.url()).toBe(connectionURL);
});

test("keeps the Basic save actions in the form instead of pinning them to the viewport", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);
  const save = page.getByRole("button", { name: "Save Basic settings" });

  await expect(save).not.toBeInViewport();
  await save.scrollIntoViewIfNeeded();
  await expect(save).toBeInViewport();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-connections-basic-save-area.png`,
    });
  }
});

test("creates a key-authenticated connection in an empty nested declared group", async ({
  page,
  installation,
}) => {
  const terminalLaunches: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/v1/terminal/sessions" && request.method() === "POST") {
      terminalLaunches.push(request.url());
    }
  });

  await openApplication(page, installation);
  await openSection(page, "Groups");
  for (const name of ["home-lab", "home-lab/others"]) {
    await page.getByLabel("New group name").fill(name);
    await page.getByRole("button", { name: "Add group" }).click();
  }
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Keys");
  await page.getByLabel("File name").fill("id_connection_e2e");
  await page.getByLabel(/Create without a passphrase/).check();
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await openSection(page, "Connections");
  await page.getByRole("button", { name: "New connection" }).click();
  const dialog = page.getByRole("dialog", { name: "Create connection" });
  await expect(dialog.getByRole("option", { name: "home-lab/others" })).toHaveCount(1);
  await dialog.getByLabel("Connection name").fill("lab-node");
  await dialog.getByLabel("Save in group").selectOption("home-lab/others");
  await dialog.getByLabel("Host name or IP address").fill("2001:db8::1");
  await dialog.getByLabel("User (optional)").fill("root");
  await dialog.getByRole("radio", { name: "SSH private key" }).check();
  const keyChoice = dialog.getByRole("combobox", { name: "SSH private key" });
  const keyID = await keyChoice.locator("option", { hasText: "id_connection_e2e" }).getAttribute("value");
  expect(keyID).not.toBeNull();
  await keyChoice.selectOption(keyID);

  expect(await clickAndAwait(page, "Create connection", "/api/v1/connections")).toBe(201);

  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "lab-node", exact: true })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
  expect(terminalLaunches).toEqual([]);
  expect(await installation.read("connections/home-lab/others/lab-node.conf")).toBe(
    "Host lab-node\n" +
    "\tHostName 2001:db8::1\n" +
    "\tUser root\n" +
    "\tPort 22\n" +
    "\tIdentityFile ~/.ssh/id_connection_e2e\n",
  );
});

test("creates a connection with a dedicated encrypted password and never starts it", async ({
  page,
  installation,
}) => {
  const password = "connection-only e2e password";
  const terminalLaunches: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/v1/terminal/sessions" && request.method() === "POST") {
      terminalLaunches.push(request.url());
    }
  });

  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("button", { name: "New connection" }).click();
  const dialog = page.getByRole("dialog", { name: "Create connection" });
  await dialog.getByLabel("Connection name").fill("password-node");
  await dialog.getByLabel("Host name or IP address").fill("password.example");
  await dialog.getByRole("textbox", { name: "Connection password", exact: true }).fill(password);

  expect(await clickAndAwait(page, "Create connection", "/api/v1/connections")).toBe(201);

  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "password-node", exact: true })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
  expect(terminalLaunches).toEqual([]);
  const config = await installation.read("config");
  expect(config).toContain(
    "Host password-node\n\tHostName password.example\n\tPort 22\n",
  );
  expect(config).not.toContain(password);
  const sealed = await installation.read("sshc/secrets");
  expect(sealed).not.toContain(password);
  expect(sealed).not.toContain("password-node");
});

test("edits a host through the form and writes only the line that changed", async ({
  page,
  installation,
}) => {
  const before = await installation.read("config");
  await openBastion(page, installation.url);

  await page.getByLabel("Port", { exact: true }).fill("2244");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);

  const after = await installation.read("config");
  expect(after).toContain("Port 2244");
  expect(after).not.toContain("Port 2222");
  expect(after).toContain("# Managed by hand since 2019. Do not reformat.");
  expect(after).toContain("HostName=203.0.113.10");
  expect(after).toContain("User    ops");
  expect(after).toContain("Include conf.d/*.conf");
  expect(after.split("\n").length).toBe(before.split("\n").length);
});

test("keeps the committed summary stable while a Basic draft crosses editor areas", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);
  const summary = page.locator("[data-connection-summary]");

  await expect(page.getByRole("heading", { name: "bastion", exact: true })).toBeVisible();
  await expect(summary.getByText("ops@203.0.113.10:2222", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2222");

  await page.getByLabel("Port", { exact: true }).fill("2244");
  await expect(summary.getByText("ops@203.0.113.10:2222", { exact: true })).toBeVisible();
  await expect(page.getByText("Unsaved changes", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Connect", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Check reachability" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Check authentication with saved settings" })).toBeDisabled();

  await page.getByRole("tab", { name: "Settings analysis" }).click();
  await page.getByRole("tab", { name: "Basic" }).click();
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2244");

  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  await expect(summary.getByText("ops@203.0.113.10:2244", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2244");
  await expect(page.getByRole("button", { name: "Check reachability" })).toBeEnabled();
});

test("keeps a Basic draft while changing group scope and asks before a connection switch", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);
  await page.getByLabel("Port", { exact: true }).fill("2244");
  const browser = page.getByRole("navigation", { name: "Connections" });

  await browser.getByRole("button", { name: "Ungrouped", exact: true }).click();
  expect(new URL(page.url()).pathname).toBe("/connections/servers");
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2244");
  await browser.getByRole("button", { name: "All", exact: true }).click();
  expect(new URL(page.url()).pathname).toBe("/connections/servers");
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2244");

  page.once("dialog", async (dialog) => dialog.dismiss());
  await browser.getByRole("button", { name: "nas" }).click();
  await expect(page.getByRole("heading", { name: "bastion", exact: true })).toBeVisible();
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2244");

  page.once("dialog", async (dialog) => dialog.accept());
  await browser.getByRole("button", { name: "nas" }).click();
  await expect(page.getByRole("heading", { name: "nas", exact: true })).toBeVisible();
});

test("saves and replaces a key-owned passphrase without changing another key's shared value", async ({
  page,
  installation,
}) => {
  const firstPassphrase = "first connection key phrase";
  const nextPassphrase = "next connection key phrase";
  await openApplication(page, installation);
  await openSection(page, "Keys");

  for (const fileName of ["id_connection_owned", "id_connection_sibling"]) {
    await page.getByLabel("File name").fill(fileName);
    await page.getByRole("textbox", { name: "Passphrase" }).fill(firstPassphrase);
    expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);
  }

  const sibling = page.getByRole("row", { name: /id_connection_sibling\b/ }).first();
  await sibling.getByRole("button", { name: "Save passphrase" }).click();
  await page.getByLabel("Passphrase name").fill("shared-sibling-phrase");
  await page.getByLabel("Passphrase value").fill(firstPassphrase);
  expect(await clickAndAwait(
    page,
    "Save and use for this key",
    "/api/v1/credentials/key_passphrase/assign",
    "PUT",
  )).toBe(200);

  const owned = page.getByRole("row", { name: /id_connection_owned\b/ }).first();
  await owned.getByRole("button", { name: "Save passphrase" }).click();
  await page.getByLabel("Use a stored passphrase").selectOption("shared-sibling-phrase");
  expect(await clickAndAwait(
    page,
    "Use this passphrase",
    "/api/v1/credentials/key_passphrase/assign",
    "PUT",
  )).toBe(200);

  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
    .click();
  const keyChoice = page.getByLabel("SSH private key");
  const ownedID = await keyChoice.locator("option", { hasText: "id_connection_owned" }).getAttribute("value");
  expect(ownedID).not.toBeNull();
  await keyChoice.selectOption(ownedID);
  await expect(page.getByText(/uses the shared saved passphrase “shared-sibling-phrase”/)).not.toBeVisible();
  await page.getByText("Save or change key passphrase").click();
  await expect(page.getByText(/uses the shared saved passphrase “shared-sibling-phrase”/)).toBeVisible();
  await page.getByLabel("New saved key passphrase", { exact: true }).fill(firstPassphrase);
  await page.getByLabel("Confirm saved key passphrase", { exact: true }).fill(firstPassphrase);
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  await expect(page.getByText("A passphrase is saved only for this key.")).not.toBeVisible();

  await openSection(page, "Secrets");
  const shared = page
    .getByRole("region", { name: "Key passphrases" })
    .getByRole("article", { name: "shared-sibling-phrase" });
  await expect(shared).toContainText("id_connection_sibling");
  await expect(shared).not.toContainText("id_connection_owned");

  await openSection(page, "Keys");
  const ownedAfterSave = page.getByRole("row", { name: /id_connection_owned\b/ }).first();
  await ownedAfterSave.getByRole("button", { name: "More actions" }).click();
  await ownedAfterSave.getByRole("button", { name: "Change passphrase" }).click();
  await page.getByLabel("Current passphrase").fill(firstPassphrase);
  await page.getByLabel("New passphrase").fill(nextPassphrase);
  expect(await clickAndAwait(page, "Save new passphrase", "/passphrase")).toBe(200);

  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
    .click();
  await expect(page.getByText("A passphrase is saved only for this key.")).not.toBeVisible();
  await page.getByText("Save or change key passphrase").click();
  await expect(page.getByText("A passphrase is saved only for this key.")).toBeVisible();
  await page.getByLabel("New saved key passphrase", { exact: true }).fill(nextPassphrase);
  await page.getByLabel("Confirm saved key passphrase", { exact: true }).fill(nextPassphrase);
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  await expect(page.getByText("A passphrase is saved only for this key.")).not.toBeVisible();
  await expect(page.locator("body")).not.toContainText(firstPassphrase);
  await expect(page.locator("body")).not.toContainText(nextPassphrase);

  const sealed = await installation.read("sshc/secrets");
  expect(sealed).not.toContain(firstPassphrase);
  expect(sealed).not.toContain(nextPassphrase);
});

test("opens a connection inside the application, with nothing to choose and nothing to install", async ({
  page,
  installation,
}) => {
  const opened: unknown[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname === "/api/v1/terminal/sessions" && request.method() === "POST") {
      opened.push(request.postDataJSON());
    }
  });
  await openApplication(page, installation);

  await openSection(page, "Settings");
  await expect(page.getByRole("region", { name: "Default connection application" })).toHaveCount(0);
  await expect(page.getByLabel("Open connections with")).toHaveCount(0);

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();
  await expect(page.getByLabel("Open with")).toHaveCount(0);
  const connect = page.getByRole("button", { name: "Connect", exact: true });
  await expect(connect).toBeEnabled();
  await connect.click();

  await expect.poll(() => opened).toEqual([{ kind: "ssh", alias: "bastion" }]);
});

test("edits the same host through Raw and keeps every other byte", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);
  await page.getByRole("tab", { name: "Advanced settings" }).click();
  await page.getByRole("tab", { name: "Raw" }).click();

  const editor = page.getByLabel(/Block text/);
  const original = await editor.inputValue();
  await editor.fill(original.replace("Port 2222", "Port 2255\n\tCompression yes"));
  expect(await clickAndAwait(page, "Save block", "/api/v1/config/save")).toBe(200);

  const after = await installation.read("config");
  expect(after).toContain("Port 2255");
  expect(after).toContain("Compression yes");
  expect(after).toContain("# Managed by hand since 2019. Do not reformat.");
  expect(after).toContain("ServerAliveInterval 30");
});

test("shows a save preview diff of exactly what was written", async ({ page, installation }) => {
  await openBastion(page, installation.url);
  await page.getByLabel("Port", { exact: true }).fill("2299");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);

  const preview = page.getByRole("region", { name: "Save preview" });
  await expect(preview).toContainText("2299");
  await expect(preview).not.toContainText("Changed on disk since you loaded it");
  expect(await installation.read("config")).toContain("Port 2299");
});

test("refuses a save whose base is stale and shows the three-way conflict", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);

  const external = (await installation.read("config")).replace(
    "Host *",
    "Host edited-outside\n\tHostName 192.0.2.99\n\nHost *",
  );
  await installation.write("config", external);

  await page.getByLabel("Port", { exact: true }).fill("2277");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(409);

  await expect(page.getByText("Changed on disk since you loaded it")).toBeVisible();
  await expect(page.getByText("Your pending change")).toBeVisible();

  const after = await installation.read("config");
  expect(after).toBe(external);
  expect(after).not.toContain("Port 2277");
});

test("shows connection checks on Basic and starts nothing unasked", async ({
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

  await openBastion(page, installation.url);

  await expect(page.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
  const panel = page.getByRole("region", { name: "Connection checks" });
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("button", { name: "Check reachability" })).toBeEnabled();
  await expect(panel.getByRole("button", { name: "Check authentication with saved settings" })).toBeEnabled();
  expect(started).toEqual([]);
});

test("shows where each value comes from without a confirmation", async ({ page, installation }) => {
  await openBastion(page, installation.url);
  await page.getByRole("tab", { name: "Settings analysis" }).click();

  await expect(page.getByRole("region", { name: "Settings analysis" })).toBeVisible();
  const show = page.getByRole("button", { name: "Show the sources" });
  await expect(show).toBeEnabled();
  await show.click();

  await expect(page.getByRole("table", { name: "Configuration lines read by OpenSSH" })).toBeVisible();
});

test("edits the display order it stores", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);

  await page.getByRole("button", { name: "Show Display and classification" }).click();

  const [ordered] = await Promise.all([
    page.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/config/save") && response.request().method() === "POST",
    ),
    page.getByLabel(/Display order/).fill("-1"),
  ]);
  expect(ordered.status()).toBe(200);
  expect(JSON.parse(await installation.read("sshc/metadata.json")).hosts[0].order).toBe(-1);
});

test("re-associates a note whose connection is gone, without guessing", async ({
  page,
  installation,
}) => {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({
      schemaVersion: 3,
      groups: [{ name: "work" }],
      hosts: [
        { identity: { path: "config", alias: "retired" }, tags: ["ci"], note: "the old builder" },
      ],
    }),
  );
  await openApplication(page, installation);
  await openSection(page, "Connections");

  const panel = page.getByRole("region", { name: "Settings without a connection" });
  await expect(panel).toBeVisible();
  await expect(panel.getByText("retired in config")).toBeVisible();
  await expect(panel.getByText(/tags ci/)).toBeVisible();

  await panel.getByLabel("Re-associate retired with").selectOption("config\u0000bastion");
  expect(await clickAndAwait(page, "Re-associate retired", "/api/v1/config/save")).toBe(200);

  const saved = JSON.parse(await installation.read("sshc/metadata.json"));
  expect(saved.hosts).toHaveLength(1);
  expect(saved.hosts[0]).toMatchObject({
    identity: { path: "config", alias: "bastion" },
    note: "the old builder",
  });
  expect(saved.hosts[0].orphan).toBeUndefined();
});

test("writes a comment into the configuration file above the Host line", async ({
  page,
  installation,
}) => {
  const before = await installation.read("config");
  await openBastion(page, installation.url);

  await page.getByRole("button", { name: "More connection actions" }).click();
  const management = page.getByRole("region", { name: "Manage connection" });
  await management.getByLabel("Comment", { exact: true }).fill("the production bastion\nask infra before changing it");
  expect(await clickAndAwait(page, "Save comment", "/api/v1/config/save")).toBe(200);

  const after = await installation.read("config");
  expect(after).toContain("# the production bastion\n# ask infra before changing it\nHost bastion\n");

  expect(after).toContain("# Managed by hand since 2019. Do not reformat.\n\nInclude conf.d/*.conf");
  expect(after.replace("# the production bastion\n# ask infra before changing it\n", "")).toBe(before);
});

test("removes the comment lines when the comment is cleared", async ({ page, installation }) => {
  const before = await installation.read("config");
  await openBastion(page, installation.url);

  await page.getByRole("button", { name: "More connection actions" }).click();
  const management = page.getByRole("region", { name: "Manage connection" });
  await management.getByLabel("Comment", { exact: true }).fill("temporary");
  expect(await clickAndAwait(page, "Save comment", "/api/v1/config/save")).toBe(200);
  await expect(management.getByLabel("Comment", { exact: true })).toHaveValue("temporary");
  await expect(management.getByRole("button", { name: "Save comment" })).toBeDisabled();

  await management.getByLabel("Comment", { exact: true }).fill("");
  expect(await clickAndAwait(page, "Save comment", "/api/v1/config/save")).toBe(200);

  expect(await installation.read("config")).toBe(before);
});

test("takes a comment with the connection it describes when the block moves", async ({
  page,
  installation,
}) => {
  await installation.write(
    "conf.d/10-home.conf",
    "# the file server\nHost nas\n\tHostName 198.51.100.20\n\n# the printer\nHost printer\n\tHostName 198.51.100.30\n",
  );
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "nas" }).click();
  await page.getByRole("button", { name: "More connection actions" }).click();
  const management = page.getByRole("region", { name: "Manage connection" });
  await expect(management.getByLabel("Comment", { exact: true })).toHaveValue("the file server");

  await page.getByLabel("Storage file").selectOption("config");
  expect(await clickAndAwait(page, "Change storage file", "/api/v1/config/save")).toBe(200);

  expect(await installation.read("config")).toContain("# the file server\nHost nas\n");
  const source = await installation.read("conf.d/10-home.conf");
  expect(source).not.toContain("the file server");
  expect(source).toContain("# the printer\nHost printer\n");
});

test("takes a comment with the connection when the block is deleted", async ({
  page,
  installation,
}) => {
  await installation.write(
    "conf.d/10-home.conf",
    "# the file server\nHost nas\n\tHostName 198.51.100.20\n\n# the printer\nHost printer\n\tHostName 198.51.100.30\n",
  );
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "nas" }).click();

  await page.getByRole("button", { name: "More connection actions" }).click();
  await page.getByRole("button", { name: "Delete connection" }).click();
  expect(await clickAndAwait(page, "Delete it", "/api/v1/config/save")).toBe(200);

  const after = await installation.read("conf.d/10-home.conf");
  expect(after).not.toContain("the file server");
  expect(after).toContain("# the printer\nHost printer\n");
});

test("shows an empty group in the management tree and moves an ungrouped connection into it", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Groups");
  await page.getByLabel("New group name").fill("work");
  await page.getByRole("button", { name: "Add group" }).click();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Connections");
  const browser = page.getByRole("navigation", { name: "Connections" });
  await expect(browser.getByRole("button", { name: "work", exact: true })).toBeVisible();

  await browser.getByRole("button", { name: "bastion" }).click();
  await page.getByRole("button", { name: "More connection actions" }).click();
  await page.getByLabel("Primary group").selectOption("work");
  expect(await clickAndAwait(page, "Move to this group", "/api/v1/config/save")).toBe(200);

  await expect(async () => {
    expect(await installation.read("connections/work/config.conf")).toContain("Host bastion");
  }).toPass();
  expect(await installation.read("config")).not.toContain("Host bastion\n");

  await expect(browser.getByRole("button", { name: "work", exact: true })).toBeVisible();
  await expect(browser.getByRole("button", { name: "bastion" })).toHaveAttribute("aria-current", "true");
});

test("opening the inspector narrows the detail rather than hiding it under the pane", async ({
  page,
  installation,
}) => {
  await openBastion(page, installation.url);
  await page.getByRole("button", { name: "Show Display and classification" }).click();
  await page.getByRole("button", { name: "More connection actions" }).click();

  const pane = page.getByRole("complementary", { name: "Display and classification" });
  await expect(pane).toBeVisible();

  const paneLeft = (await pane.boundingBox())?.x ?? 0;
  expect(paneLeft).toBeGreaterThan(0);

  for (const name of [
    "Duplicate connection",
    "Change storage file",
    "Delete connection",
    "Save Basic settings",
  ]) {
    const box = await page.getByRole("button", { name, exact: true }).boundingBox();
    expect(box, `${name} has no box`).not.toBeNull();
    expect(box!.x + box!.width, `${name} runs under the inspector`).toBeLessThanOrEqual(paneLeft);
  }
});

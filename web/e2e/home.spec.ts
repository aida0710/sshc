import { expect, openApplication, openSection, test } from "./support/environment";
import { metadataSchemaVersion } from "./support/settings";
import { terminalRoot } from "./support/terminal";

test("starts with a searchable host launcher and contacts nothing unasked", async ({
  page,
  installation,
}) => {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({
      schemaVersion: metadataSchemaVersion,
      hosts: [
        {
          identity: { path: "config", alias: "bastion" },
          tags: ["production"],
        },
      ],
    }),
  );
  await installation.write(
    "sshc/recent-connections.json",
    JSON.stringify({
      schemaVersion: 1,
      entries: [{ alias: "bastion", lastConnectedAt: "2026-08-24T15:30:00Z" }],
    }),
  );

  const terminalRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/v1/terminal/")) return;
    if (request.method() === "GET") return;
    terminalRequests.push(`${request.method()} ${path}`);
  });

  await openApplication(page, installation);

  await expect(page.getByRole("heading", { name: "Home" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  const launcher = page.getByRole("list", { name: "Available connections" });
  const bastion = launcher.getByRole("listitem").filter({ hasText: "bastion" });
  await expect(bastion).toBeVisible();
  await expect(bastion.getByText("ops@203.0.113.10:2222", { exact: true })).toBeVisible();
  // The last connection is a badge (2 months ago); its name holds the whole time.
  await expect(bastion.getByLabel(/^Last connected /u)).toBeVisible();
  await expect(page.getByText("nas", { exact: true })).toBeVisible();
  expect(terminalRequests).toEqual([]);

  await page.getByRole("searchbox", { name: "Search connections" }).fill("production");
  await expect(page.getByRole("list", { name: "Available connections" }).getByText(/bastion/)).toBeVisible();
  await expect(page.getByText("nas", { exact: true })).toHaveCount(0);
  expect(terminalRequests).toEqual([]);

  await page.getByRole("button", { name: "Manage connections" }).click();
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();
});

test("offers the ways to begin while there is no connection, and each tile goes there", async ({
  page,
  installation,
}) => {
  await installation.write("config", "");
  await openApplication(page, installation);
  await openSection(page, "Home");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Get started" })).toBeVisible();

  await main.getByRole("button", { name: /^Open SSH Config/ }).click();
  await expect(page).toHaveURL(/\/config/);

  await openSection(page, "Home");
  await main.getByRole("button", { name: /^Receive by sync/ }).click();
  await expect(page).toHaveURL(/\/sync/);

  await openSection(page, "Home");
  await main.getByRole("button", { name: /^Add connection/ }).click();
  await expect(page.getByRole("form", { name: "New connection" })).toBeVisible();

  await openSection(page, "Home");
  await main.getByRole("button", { name: "Open a local shell", exact: true }).click();
  await expect(page).toHaveURL(/\/terminal/);
  await expect(terminalRoot(page)).toBeVisible();
});

test("asks for attention to a line OpenSSH cannot read and shows it in SSH Config", async ({
  page,
  installation,
}) => {
  await installation.write("conf.d/20-typo.conf", "Host typo-host\n\tHostNmae 192.0.2.77\n\tUser deploy\n");
  await openApplication(page, installation);

  const attention = page.getByRole("region", { name: "Needs attention" });
  await expect(attention).toContainText("The SSH configuration files have 1 problem(s).");
  await attention.getByRole("button", { name: "Open SSH Config" }).click();

  await expect(page.getByRole("heading", { name: "Diagnostics: 1" })).toBeVisible();
  await expect(page.getByText(
    "OpenSSH cannot read this line (HostNmae). While it is there, OpenSSH reads none of the configuration " +
      "and the ssh command stops working (conf.d/20-typo.conf:2)",
    { exact: true },
  )).toBeVisible();
});

test("requires a double click for a mouse", async ({
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

  const bastion = page.getByRole("button", { name: /^Connect to bastion\./ });
  await bastion.click();
  expect(opened).toEqual([]);
  await bastion.dblclick();
  await expect.poll(() => opened).toEqual([{ kind: "ssh", alias: "bastion" }]);
});

test("opens the action menu without connecting, then keeps settings and connect distinct", async ({
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

  await page.getByRole("button", { name: "Actions for bastion" }).click();
  expect(opened).toEqual([]);
  await page.getByRole("menuitem", { name: "Open connection settings" }).click();
  await expect(page).toHaveURL(/\/connections\/servers\?path=config&host=bastion&panel=basic$/);
  await expect(page.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
  expect(opened).toEqual([]);

  await openSection(page, "Home");
  await page.getByRole("button", { name: "Actions for bastion" }).click();
  await page.getByRole("menuitem", { name: "Connect", exact: true }).click();
  await expect.poll(() => opened).toEqual([{ kind: "ssh", alias: "bastion" }]);
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 411, height: 914 }, hasTouch: true, isMobile: true });

  test("keeps many groups on one line that scrolls, so the connections stay on the first screen", async ({ page, installation }) => {
    const groups = Array.from({ length: 14 }, (_, index) => `group-${index + 1}`);
    await installation.write("config", [
      "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.",
      "# Edit through the UI; lines between these markers are replaced on the next save.",
      ...groups.map((name) => `Include connections/${name}/*.conf`),
      "Include groups.sshc.conf",
      "# <<< sshc groups",
      "",
    ].join("\n"));
    await installation.write("groups.sshc.conf", "");
    for (const name of groups) {
      await installation.write(`connections/${name}/hosts.conf`, `Host ${name}-web\n\tHostName 198.51.100.1\n`);
    }
    await openApplication(page, installation);

    const row = page.getByRole("group", { name: "Filter connections by group" });
    await expect(row.getByRole("button")).toHaveCount(groups.length);
    const tops = await row.getByRole("button").evaluateAll((chips) => chips.map((chip) => chip.getBoundingClientRect().top));
    expect(new Set(tops).size).toBe(1);
    expect(await row.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await expect(page.getByRole("list", { name: "Available connections" }).getByRole("listitem").first()).toBeInViewport();
  });
});

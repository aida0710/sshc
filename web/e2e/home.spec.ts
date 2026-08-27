import { expect, openApplication, test } from "./support/environment";

test("starts with a searchable host launcher and contacts nothing unasked", async ({
  page,
  installation,
}) => {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({
      schemaVersion: 3,
      hosts: [
        {
          identity: { path: "config", alias: "bastion" },
          tags: ["production"],
          favourite: true,
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

  await expect(page.getByRole("heading", { name: "Your connections" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  const launcher = page.getByRole("list", { name: "Available connections" });
  const bastion = launcher.getByRole("listitem").filter({ hasText: "bastion" });
  await expect(bastion).toBeVisible();
  await expect(bastion.getByText("ops@203.0.113.10:2222", { exact: true })).toBeVisible();
  await expect(bastion).toContainText("Last connected");
  await expect(page.getByText("nas", { exact: true })).toBeVisible();
  expect(terminalRequests).toEqual([]);

  await page.getByRole("searchbox", { name: "Search connections" }).fill("production");
  await expect(page.getByRole("list", { name: "Available connections" }).getByText(/bastion/)).toBeVisible();
  await expect(page.getByText("nas", { exact: true })).toHaveCount(0);
  expect(terminalRequests).toEqual([]);

  await page.getByRole("button", { name: "Manage connections" }).click();
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();
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

  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Home" }).click();
  await page.getByRole("button", { name: "Actions for bastion" }).click();
  await page.getByRole("menuitem", { name: "Connect", exact: true }).click();
  await expect.poll(() => opened).toEqual([{ kind: "ssh", alias: "bastion" }]);
});

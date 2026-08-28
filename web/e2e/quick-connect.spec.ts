import { expect, openApplication, test } from "./support/environment";

type GroupFixture = {
  name: string;
  aliases: string[];
};

const groups: GroupFixture[] = [
  { name: "Production/Asia", aliases: ["tokyo-1", "tokyo-2", "singapore", "seoul"] },
  { name: "Production/Europe", aliases: ["frankfurt", "london", "paris"] },
  { name: "Production", aliases: ["prod-bastion", "prod-control", "prod-db"] },
  { name: "Development", aliases: ["dev-api", "dev-db", "dev-worker", "dev-cache", "dev-ci"] },
  { name: "Network", aliases: ["core-router", "firewall", "vpn", "dns"] },
  { name: "Home", aliases: ["nas", "home-lab"] },
  { name: "Staging", aliases: ["stage-api", "stage-db", "stage-worker"] },
  { name: "Clients", aliases: ["client-a", "client-b"] },
  { name: "Edge", aliases: ["edge-east", "edge-west"] },
  { name: "Archive", aliases: ["archive-node"] },
];

function connectionFile(aliases: string[], firstAddress: number): string {
  return aliases.map((alias, position) => [
    `Host ${alias}`,
    `\tHostName 198.51.100.${String(firstAddress + position)}`,
    "\tUser operator",
    "\tPort 22",
    "",
  ].join("\n")).join("\n");
}

async function seedQuickConnect(installation: {
  write(relative: string, contents: string): Promise<void>;
}) {
  const includes = groups.map((group) => `Include connections/${group.name}/*.conf`).join("\n");
  await installation.write("config", [
    "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.",
    "# Edit through the UI; lines between these markers are replaced on the next save.",
    includes,
    "Include groups.sshc.conf",
    "# <<< sshc groups",
    "",
    "Host *",
    "\tServerAliveInterval 30",
    "",
  ].join("\n"));
  let address = 10;
  for (const group of groups) {
    await installation.write(
      `connections/${group.name}/hosts.conf`,
      connectionFile(group.aliases, address),
    );
    address += group.aliases.length;
  }
}

test("drills through group levels and keeps every descendant connection in scope", async ({
  page,
  installation,
}) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await seedQuickConnect(installation);
  await openApplication(page, installation);

  const rootGroups = page.getByRole("group", { name: "Filter connections by group" });
  await expect(page.getByRole("heading", { name: "Groups 8" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Connections 29" })).toBeVisible();
  await expect(rootGroups.getByRole("button")).toHaveCount(8);
  await expect(rootGroups.getByRole("button", { name: "Open Production, 10 connections" })).toBeVisible();
  await expect(rootGroups.getByRole("button", { name: "Open Production/Asia, 4 connections" })).toHaveCount(0);

  const grid = await rootGroups.evaluate((element) => ({
    display: getComputedStyle(element).display,
    columns: getComputedStyle(element).gridTemplateColumns.split(" ").length,
    scrollWidth: element.scrollWidth,
    clientWidth: element.clientWidth,
  }));
  expect(grid).toMatchObject({ display: "grid", columns: 4 });
  expect(grid.scrollWidth).toBeLessThanOrEqual(grid.clientWidth);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-quick-connect-groups-root.png`,
    });
  }

  await rootGroups.getByRole("button", { name: "Open Production, 10 connections" }).click();
  const productionGroups = page.getByRole("group", { name: "Filter connections by group" });
  await expect(page.getByRole("heading", { name: "Groups 2" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Connections 10" })).toBeVisible();
  await expect(productionGroups.getByRole("button")).toHaveCount(2);
  await expect(productionGroups.getByRole("button", { name: "Open Production/Asia, 4 connections" })).toBeVisible();
  await expect(productionGroups.getByRole("button", { name: "Open Production/Europe, 3 connections" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Selected group" })).toContainText("All/Production");
  await expect(page.getByText("prod-bastion", { exact: true })).toBeVisible();
  await expect(page.getByText("tokyo-1", { exact: true })).toBeVisible();
  await expect(page.getByText("dev-api", { exact: true })).toHaveCount(0);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-quick-connect-groups-production.png`,
    });
  }
});

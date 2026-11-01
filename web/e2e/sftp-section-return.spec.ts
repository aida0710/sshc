import { expect, openApplication, openSection, test } from "./support/environment";
import { keepMachineState } from "./support/settings";

const report = {
  name: "report.txt", path: "/srv/report.txt", type: "file", size: 4096, mode: "0644",
  modifiedAt: "2026-10-10T00:00:00Z", revision: "report-v1",
};

// Going to another section takes the SFTP panes down. A tab the person connected comes back connected; after a
// reload, a restored tab still waits for Connect (tabsConnectedOnLeaving.ts).
test("a tab connected before going to another section is connected again on coming back, but not after a reload", async ({ page, installation }) => {
  let remoteReads = 0;
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => {
    remoteReads += 1;
    return route.fulfill({ json: { path: "/srv", entries: [report] } });
  });
  await openApplication(page, installation);
  await keepMachineState(page, "sftpPanes", JSON.stringify([{ tabs: [{ alias: "bastion", path: "/srv" }], activeIndex: 0 }]));
  await page.reload();
  await openSection(page, "SFTP");
  const pane = page.getByRole("tabpanel");
  await pane.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(pane.getByRole("button", { name: "report.txt", exact: true })).toBeVisible();

  await openSection(page, "Home");
  const readsBeforeReturn = remoteReads;
  await openSection(page, "SFTP");
  await expect(pane.getByRole("button", { name: "report.txt", exact: true })).toBeVisible();
  await expect(pane.getByRole("button", { name: "Connect", exact: true })).toHaveCount(0);
  expect(remoteReads).toBeGreaterThan(readsBeforeReturn);

  await page.reload();
  await openSection(page, "SFTP");
  await expect(pane.getByRole("button", { name: "Connect", exact: true })).toBeVisible();
});

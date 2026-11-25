import type { RemoteEntry } from "../src/sftp/api";
import { expect, openApplication, test } from "./support/environment";
import { chooseEntryOnPhone, connectSFTPHost } from "./support/sftp";

const entries: RemoteEntry[] = [
  { name: "notes.txt", path: "/work/notes.txt", type: "file", size: 2048, mode: "0644", modifiedAt: "2026-10-07T06:00:00Z", revision: "file-v1" },
];

// A page of the phone's bottom bar stays mounted while another is shown, so a failure reported in it is still there
// on coming back, and nothing but the next action of that page would take it away.
test("a failure left in SFTP is there after visiting Home, and its × hides it", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: () => Promise.reject(new Error("denied")) } });
    document.execCommand = () => false;
  });
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({ json: { path: "/work", entries } }));
  await openApplication(page, installation);
  const bar = page.getByRole("navigation", { name: "Quick navigation" });
  await bar.getByRole("link", { name: "SFTP", exact: true }).click();
  await connectSFTPHost(page, "bastion");
  const pane = page.getByRole("tabpanel").first();
  await chooseEntryOnPhone(pane, "notes.txt");
  await pane.getByRole("button", { name: "Actions for notes.txt", exact: true }).click();
  await page.getByRole("menuitem", { name: "Copy full path", exact: true }).click();
  const failure = pane.getByRole("alert");
  await expect(failure).toHaveText("The browser refused to write to the clipboard.");

  await bar.getByRole("link", { name: "Home", exact: true }).click();
  await expect(pane).toBeHidden();
  await bar.getByRole("link", { name: "SFTP", exact: true }).click();
  await expect(failure).toBeVisible();
  await failure.getByRole("button", { name: "Hide this notice", exact: true }).click();

  await expect(failure).toHaveCount(0);
  await expect(pane.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();
});

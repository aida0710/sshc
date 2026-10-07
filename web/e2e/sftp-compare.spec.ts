import { join } from "node:path";
import { expect, openApplication, openSection, test } from "./support/environment";
import { openSecondSFTPPane } from "./support/sftp";

test("compares engine-local directory metadata without offering a write operation", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1800, height: 900 });
  await installation.write("compare-left/only-left.txt", "left");
  await installation.write("compare-left/changed.txt", "short");
  await installation.write("compare-right/only-right.txt", "right");
  await installation.write("compare-right/changed.txt", "a different length");
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && new URL(request.url()).pathname.startsWith("/api/v1/sftp/")) {
      mutations.push(request.url());
    }
  });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openSecondSFTPPane(page, "New tab");
  for (const [index, directory] of ["compare-left", "compare-right"].entries()) {
    const pane = page.getByRole("tabpanel").nth(index);
    await pane.locator("button[data-value]:visible").click();
    await page.getByRole("dialog").getByText("Local", { exact: true }).click();
    await pane.getByRole("button", { name: "Edit local path", exact: true }).click();
    const pathInput = pane.getByRole("textbox", { name: "Engine filesystem path", exact: true });
    await pathInput.fill(join(installation.home, ".ssh", directory));
    await pathInput.press("Enter");
    await expect(pane.getByRole("button", { name: "changed.txt", exact: true })).toBeVisible();
  }

  await page.getByRole("button", { name: "Compare directories", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Compare directories", exact: true });
  await expect(dialog).toContainText("Files are left unchanged.");
  await expect(dialog.getByRole("row", { name: /only-left.txt/ })).toContainText("Left only");
  await expect(dialog.getByRole("row", { name: /only-right.txt/ })).toContainText("Right only");
  await expect(dialog.getByRole("row", { name: /changed.txt/ })).toContainText("Changed");
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: /^Copy / })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(mutations).toEqual([]);
  expect(await installation.read("compare-left/changed.txt")).toBe("short");
  expect(await installation.read("compare-right/changed.txt")).toBe("a different length");
});

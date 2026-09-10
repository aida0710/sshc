import { mkdir } from "node:fs/promises";
import { expect, openApplication, test } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

test("shows OS icons and editable paste and close dialogs with synthetic connections", async ({ page, context, installation }) => {
  const hosts = [
    ["demo-ubuntu", "ubuntu"], ["demo-redhat", "redhat"], ["demo-debian", "debian"],
    ["demo-macos", "macos"], ["demo-windows", "windows"], ["demo-unknown", ""],
  ];
  await installation.write("config", hosts.map(([alias]) => `Host ${alias}\n  HostName 192.0.2.10\n  User demo\n`).join("\n"));
  await installation.write("sshc/metadata.json", JSON.stringify({ schemaVersion: 4, hosts: hosts.map(([alias, os]) => ({ identity: { path: "config", alias }, os })) }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  for (const name of ["Ubuntu", "Red Hat Enterprise Linux", "Debian", "macOS", "Windows", "OS unknown"]) {
    await expect(page.getByRole("img", { name, exact: true })).toBeVisible();
  }
  await expect(page.getByRole("img", { name: "Ubuntu", exact: true })).toHaveCSS("width", "24px");
  await expect(page.getByRole("img", { name: "Ubuntu", exact: true }).locator("span").first()).toHaveCSS("width", "20px");
  const dir = process.env.SSHC_UI_VISUAL_DIR;
  if (dir !== undefined) {
    await mkdir(dir, { recursive: true });
    await page.screenshot({ path: `${dir}/home-os-icons-monochrome.png`, fullPage: true });
  }
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "Local shell" }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toContainText(/[$#%>]/);
  await page.evaluate(() => navigator.clipboard.writeText("echo example-one\necho example-two\n"));
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Shift+V");
  const editor = page.getByRole("textbox", { name: "Edit paste" });
  await editor.fill("echo edited-example\necho second-example\n");
  if (dir !== undefined && process.env.SSHC_UI_HOME_ONLY !== "1") await page.screenshot({ path: `${dir}/editable-paste.png`, fullPage: true });
  await page.getByRole("dialog").getByRole("button", { name: "Cancel" }).click();
  const row = page.getByRole("list", { name: "Open consoles" }).getByRole("listitem").first();
  await row.getByRole("button", { name: /^Close / }).click();
  await expect(page.getByRole("button", { name: "Keep it open" })).toBeFocused();
  if (dir !== undefined && process.env.SSHC_UI_HOME_ONLY !== "1") await page.screenshot({ path: `${dir}/close-dialog-focus.png`, fullPage: true });
});

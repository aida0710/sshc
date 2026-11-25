import { expect, openApplication, openLocalShell, openSection, test } from "./support/environment";

test("keeps the name being typed in the sidebar when the terminal appears a moment after its session opened", async ({ page, installation }) => {
  let showTerminal: () => void = () => {};
  const terminalHeld = new Promise<void>((resolve) => { showTerminal = resolve; });
  await page.route(/\/assets\/TerminalView-[^/]+\.js$/, async (route) => {
    await terminalHeld;
    await route.continue();
  });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await openLocalShell(page);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Rename", exact: true }).click();
  const name = navigation.getByRole("textbox", { name: /^New name for / });
  await expect(name).toBeFocused();

  showTerminal();
  const terminal = page.getByRole("region", { name: /^Terminal for / });
  await expect(terminal.getByRole("textbox", { name: "Terminal input" })).toBeAttached();
  await expect(terminal.getByText("connected", { exact: true })).toBeVisible();
  await expect(name).toBeFocused();
  await name.fill("build server");
  await name.press("Enter");
  await expect(navigation.getByRole("button", { name: "build server", exact: true })).toBeVisible();
});

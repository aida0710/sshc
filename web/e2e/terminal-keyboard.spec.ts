import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

for (const mode of [
  { name: "legacy", enable: "", bytes: 5, hex: "091b5b5a78" },
  { name: "kitty", enable: "\\033[>1u", bytes: 8, hex: "091b5b393b327578" },
]) {
  test(`sends Tab and Shift+Tab to a real shell without moving focus in ${mode.name} mode`, async ({ page, installation }) => {
    test.skip(process.platform === "win32", "stty and od require a POSIX shell");
    await installation.write("keyboard.sh", [
      "saved=$(stty -g)",
      "trap 'stty \"$saved\"; printf \"\\033[<u\"' EXIT",
      "stty raw -echo",
      `printf '${mode.enable}keyboard-ready\\r\\n'`,
      `received=$(dd bs=1 count=${mode.bytes} 2>/dev/null | od -An -tx1 | tr -d ' \\n')`,
      "printf 'keys:%s\\r\\n' \"$received\"",
      "",
    ].join("\n"));
    await openApplication(page, installation);
    await openLocalShell(page);
    const screen = page.getByRole("region", { name: /^Terminal for / });
    await expect(screen).toContainText(/[$#%>]/);
    const keyboard = terminalKeyboard(page);
    await keyboard.focus();
    await page.keyboard.type("sh ~/.ssh/keyboard.sh");
    await page.keyboard.press("Enter");
    await expect(screen).toContainText("keyboard-ready");

    await page.keyboard.press("Tab");
    await expect(keyboard).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(keyboard).toBeFocused();
    // A following printable key detects duplicate escape sequences as well.
    await page.keyboard.type("x");
    await expect(screen).toContainText(`keys:${mode.hex}`);

    await page.keyboard.press("Control+k");
    await expect(page.getByRole("searchbox", { name: "Search sessions, hosts, files, snippets and settings" })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(keyboard).toBeFocused();

    await page.getByRole("button", { name: "New workspace", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "New workspace", exact: true });
    const name = dialog.getByRole("textbox", { name: "Workspace name" });
    await expect(name).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(dialog.getByRole("button", { name: "Create", exact: true })).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(name).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(page.getByRole("button", { name: "New workspace", exact: true })).toBeFocused();
  });
}

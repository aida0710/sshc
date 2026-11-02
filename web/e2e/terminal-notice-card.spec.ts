import { expect, openApplication, openLocalShell, openSection, test } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

test("a shell that fails puts a card over the terminal that can be hidden, and it stays hidden when shown again", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await terminalKeyboard(page).first().focus();
  await page.keyboard.type("exit 3\n");

  const card = screen.getByRole("region", { name: "The program exited with status 3." });
  await expect(card).toBeVisible();
  // The strip above says it too, so hiding the card loses nothing.
  await expect(screen.getByText("The program exited with status 3.")).toHaveCount(2);
  await card.getByRole("button", { name: "Hide this notice" }).click();
  await expect(card).toBeHidden();
  await expect(screen.getByText("The program exited with status 3.")).toHaveCount(1);

  // Hidden stays hidden when the terminal is shown again.
  await openSection(page, "Home");
  await openSection(page, "Terminal");
  await expect(screen.getByText("The program exited with status 3.")).toHaveCount(1);
  await expect(card).toHaveCount(0);
});

test("the card of a failed shell closes the session", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await terminalKeyboard(page).first().focus();
  await page.keyboard.type("exit 3\n");
  const list = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" });
  const sessionsBefore = await list.getByRole("listitem").count();

  await screen.getByRole("region", { name: "The program exited with status 3." }).getByRole("button", { name: "Close", exact: true }).click();
  await expect(list.getByRole("listitem")).toHaveCount(sessionsBefore - 1);
});

test("a clean exit stays in the strip above, without a card", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await terminalKeyboard(page).first().focus();
  await page.keyboard.type("exit\n");

  await expect(screen.getByText("The program exited with status 0.")).toBeVisible();
  await expect(screen.getByRole("button", { name: "Hide this notice" })).toHaveCount(0);
});

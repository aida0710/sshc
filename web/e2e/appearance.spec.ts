import { expect, test } from "./support/environment";
import { openApplication, openSection, sessionStatus } from "./support/environment";

const sections = [
  "Home",
  "Connections",
  "SFTP",
  "Snippets",
  "Config",
  "Groups",
  "Keys",
  "Known Hosts",
  "Install Key on Server",
  "Ad hoc checks",
  "Secrets",
  "Sync",
  "History",
];

for (const appearance of ["light", "dark"] as const) {
  test(`every section renders in ${appearance}`, async ({ page, installation }) => {
    await openApplication(page, installation);

    await expect(page.locator("[data-session-status-badge]")).toBeVisible();
    await page.getByLabel("Appearance").selectOption(appearance);
    await expect(page.locator("html")).toHaveAttribute("data-theme", appearance);
    await expect(sessionStatus(page)).toContainText("Local session active");

    for (const name of sections) {
      await openSection(page, name);

      await expect(page.locator("html")).toHaveAttribute("data-theme", appearance);
      await expect(page.locator("main")).toBeVisible();

      const painted = await page.evaluate(() => {
        const shell = document.querySelector("main");
        if (shell === null) return null;
        const style = window.getComputedStyle(shell);
        const body = window.getComputedStyle(document.body);
        return { colour: style.color, background: body.backgroundColor };
      });
      expect(painted).not.toBeNull();
      expect(painted?.colour).not.toBe(painted?.background);
      expect(painted?.colour).not.toBe("rgba(0, 0, 0, 0)");
    }

    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-appearance-${appearance}.png`,
        fullPage: true,
      });
    }
  });

  test(`structural borders and small supporting text keep their contrast in ${appearance}`, async ({ page, installation }) => {
    await openApplication(page, installation);
    await page.getByLabel("Appearance").selectOption(appearance);

    const contrast = await page.evaluate(() => {
      const root = getComputedStyle(document.documentElement);
      const rgb = (name: string) => {
        const value = root.getPropertyValue(name).trim();
        const match = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(value);
        if (match === null) throw new Error(`unexpected colour ${name}: ${value}`);
        return match.slice(1).map((part) => Number.parseInt(part, 16) / 255);
      };
      const luminance = (colour: number[]) => {
        const linear = colour.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
        return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!;
      };
      const ratio = (left: string, right: string) => {
        const values = [luminance(rgb(left)), luminance(rgb(right))].sort((a, b) => b - a);
        return (values[0]! + 0.05) / (values[1]! + 0.05);
      };
      return {
        cardBorder: ratio("--ui-control-line", "--ui-card"),
        controlBorder: ratio("--ui-control-line", "--ui-control"),
        faintOnCard: ratio("--ui-ink-faint", "--ui-card"),
        faintOnSidebar: ratio("--ui-ink-faint", "--ui-sidebar"),
      };
    });

    expect(contrast.cardBorder).toBeGreaterThanOrEqual(3);
    expect(contrast.controlBorder).toBeGreaterThanOrEqual(3);
    expect(contrast.faintOnCard).toBeGreaterThanOrEqual(4.5);
    expect(contrast.faintOnSidebar).toBeGreaterThanOrEqual(4.5);
  });
}

test("the connections controls are legible in light", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.getByLabel("Appearance").selectOption("light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await openSection(page, "Connections");
  await page.getByRole("button", { name: "bastion", exact: true }).click();
  await expect(page.getByRole("heading", { name: "bastion", level: 2 })).toBeVisible();
  await page.getByRole("button", { name: "New connection" }).click();

  const readable = await page.evaluate(() => {
    const results: { where: string; colour: string; background: string }[] = [];
    for (const selector of [
      "input#create-connection-name",
      "select#create-connection-group",
      "input#create-connection-hostname",
    ]) {
      const element = document.querySelector(selector);
      if (element === null) continue;
      const style = window.getComputedStyle(element);
      results.push({ where: selector, colour: style.color, background: style.backgroundColor });
    }
    return results;
  });

  expect(readable.map((control) => control.where)).toEqual([
    "input#create-connection-name",
    "select#create-connection-group",
    "input#create-connection-hostname",
  ]);
  for (const control of readable) {
    expect(control.colour, `${control.where} text`).not.toBe(control.background);
    expect(control.background, `${control.where} background`).not.toBe("rgb(28, 28, 30)");
  }
});

import { codeField } from "./support/codeField";
import { chooseConnectionAction } from "./support/connections";
import { expect, test } from "./support/environment";
import { chooseTheme, openApplication, openSection, sessionStatus, themeChoiceLabels } from "./support/environment";
import type { ColorTheme, Theme } from "../src/theme/theme";
import { everySection } from "./support/sectionNavigation";
import { answerNoVPNProfiles } from "./support/vpn";

const everyTheme = (Object.keys(themeChoiceLabels) as Theme[]).filter((theme): theme is ColorTheme => theme !== "system");

for (const appearance of everyTheme) {
  test(`every section renders in ${appearance}`, async ({ page, installation }) => {
    await answerNoVPNProfiles(page);
    await openApplication(page, installation);

    await expect(page.locator("[data-session-status-badge]")).toBeVisible();
    await chooseTheme(page, appearance);
    await openSection(page, "Menu");
    const displaySettings = page.getByRole("region", { name: "Menu" });
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      if (process.env.SSHC_VISUAL_LOCALE === "ja") {
        await displaySettings.getByLabel("Language").selectOption("ja");
        await page.evaluate(async () => { await document.fonts.ready; });
      }
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/display-settings-desktop-${process.env.SSHC_VISUAL_LOCALE === "ja" ? "ja" : appearance}.png`,
        fullPage: true,
      });
      if (process.env.SSHC_VISUAL_LOCALE === "ja") {
        await displaySettings.getByLabel("Language").selectOption("en");
      }
    }
    await expect(page.locator("html")).toHaveAttribute("data-theme", appearance);
    await expect(sessionStatus(page)).toContainText("Local session active");

    for (const { navigation } of everySection) {
      await openSection(page, navigation);

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
    await chooseTheme(page, appearance);

    const contrast = await page.evaluate(() => {
      // Colours are read as painted: one made with color-mix, like the selection fill, is no number as a variable.
      const probe = document.createElement("span");
      document.body.append(probe);
      const rgb = (name: string) => {
        probe.style.color = `var(${name})`;
        const painted = getComputedStyle(probe).color;
        const channels = /^rgb\((\d+), (\d+), (\d+)\)$/.exec(painted)?.slice(1).map((part) => Number(part) / 255)
          ?? /^color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)\)$/.exec(painted)?.slice(1).map(Number);
        if (channels === undefined) throw new Error(`unexpected colour ${name}: ${painted}`);
        return channels;
      };
      const luminance = (colour: number[]) => {
        const linear = colour.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
        return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!;
      };
      const ratio = (left: string, right: string) => {
        const values = [luminance(rgb(left)), luminance(rgb(right))].sort((a, b) => b - a);
        return (values[0]! + 0.05) / (values[1]! + 0.05);
      };
      const ratios = {
        cardBorder: ratio("--ui-line", "--ui-card"),
        controlBorder: ratio("--ui-control-line", "--ui-control"),
        faintOnCard: ratio("--ui-ink-faint", "--ui-card"),
        faintOnSidebar: ratio("--ui-ink-faint", "--ui-sidebar"),
        faintOnSelection: ratio("--ui-ink-faint", "--ui-select-fill"),
        accentOnCard: ratio("--ui-accent", "--ui-card"),
        primaryButton: ratio("--ui-accent-ink", "--ui-accent"),
      };
      probe.remove();
      return ratios;
    });

    expect(contrast.cardBorder).toBeGreaterThanOrEqual(1.2);
    expect(contrast.controlBorder).toBeGreaterThanOrEqual(2.5);
    expect(contrast.faintOnCard).toBeGreaterThanOrEqual(4.5);
    expect(contrast.faintOnSidebar).toBeGreaterThanOrEqual(4.5);
    expect(contrast.faintOnSelection).toBeGreaterThanOrEqual(4.5);
    expect(contrast.accentOnCard).toBeGreaterThanOrEqual(4.5);
    expect(contrast.primaryButton).toBeGreaterThanOrEqual(4.5);

    await openSection(page, "Connections");
    await page
      .getByRole("navigation", { name: "Connections" })
      .getByRole("button", { name: "bastion", exact: true })
      .click();
    const summary = page.locator("[data-connection-summary]");
    await expect(page.getByRole("tablist", { name: "Connection editor" })).toHaveCSS("border-bottom-width", "1px");
    await expect(summary.getByRole("heading")).toBeVisible();
    await expect(summary.getByRole("button", { name: "Connect", exact: true })).toBeVisible();
    await expect(page.locator("[data-connection-editor] input").first()).toHaveCSS("border-radius", "8px");
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-summary-${appearance}.png`,
        fullPage: true,
      });
    }

    await page.getByRole("tab", { name: "Resolved values" }).click();
    await expect(page.getByRole("tabpanel", { name: "Resolved values" })).toBeVisible();
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-analysis-${appearance}.png`,
        fullPage: true,
      });
    }

    await page.getByRole("tab", { name: "Advanced" }).click();
    await expect(page.getByRole("tabpanel", { name: "Advanced" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Advanced" })).toHaveAttribute("aria-selected", "true");
    await page.waitForTimeout(250);
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-advanced-jump-${appearance}.png`,
        fullPage: true,
      });
    }
    for (const subview of ["raw"] as const) {
      await chooseConnectionAction(page, "Edit as text");
      await expect(page.getByLabel(/Block text/)).toBeVisible();
      await page.waitForTimeout(250);
      if (process.env.SSHC_VISUAL_DIR !== undefined) {
        await page.screenshot({
          path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-advanced-${subview}-${appearance}.png`,
          fullPage: true,
        });
      }
    }

    await openSection(page, "Ad hoc checks");
    // The page's sections are divided by a rule. Waiting for one that the loaded page shows keeps the
    // loading skeleton's cards from being measured instead.
    const configuration = page.getByRole("region", { name: /^Configuration/ });
    await expect(configuration).toBeVisible();
    await expect(configuration).toHaveCSS("border-top-width", "1px");
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-diagnostics-${appearance}.png`,
        fullPage: true,
      });
    }

    await openSection(page, "Config");
    // The editor is the one framed area in the Config workspace, so its edge has to show in every theme.
    const editor = codeField(page, /File text/);
    await expect(editor).toHaveCSS("border-top-width", "1px");
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-config-explorer-${appearance}.png`,
        fullPage: true,
      });
    }
  });
}

test("the keys in the key bar's settings stand out from what is behind them in every theme", async ({ page, installation }) => {
  await openApplication(page, installation);
  for (const appearance of everyTheme) {
    await chooseTheme(page, appearance);
    await openSection(page, "Key bar");
    const colours = await page.locator("[data-key-bar-preview]").evaluate((preview) => ({
      behind: getComputedStyle(preview).backgroundColor,
      key: getComputedStyle(preview.querySelector("button")!).backgroundColor,
    }));
    expect(colours.key, appearance).not.toBe(colours.behind);
  }
});

test("the connections controls are legible in light", async ({ page, installation }) => {
  await openApplication(page, installation);
  await chooseTheme(page, "light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await openSection(page, "Connections");
  await page.getByRole("button", { name: "bastion", exact: true }).click();
  await expect(page.getByRole("heading", { name: "bastion", level: 3 })).toBeVisible();
  await page.getByRole("button", { name: "Add connection" }).click();

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

test("a field in focus is marked by the accent outline alone, as a button is", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("button", { name: "bastion", exact: true }).click();
  const field = page.locator("[data-connection-editor] input").first();
  const borderAtRest = await field.evaluate((element) => getComputedStyle(element).borderTopColor);

  await field.focus();

  await expect(field).toHaveCSS("outline-style", "solid");
  await expect(field).toHaveCSS("outline-width", "2px");
  await expect(field).toHaveCSS("outline-offset", "2px");
  // Neither a ring nor a second line in the accent is drawn inside the outline.
  await expect(field).toHaveCSS("box-shadow", "none");
  await expect(field).toHaveCSS("border-top-color", borderAtRest);
});

// A row that reaches the edge of a box that hides its overflow would lose one side of the outline, 4px outside it.
test("a row at the edge of its box draws the outline of focus inside itself", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openApplication(page, installation);
  await openSection(page, "Connections");
  const group = page.getByRole("navigation", { name: "Groups" }).getByRole("button", { name: /Ungrouped/ }).first();

  // A press on the keyboard comes first, so the browser shows the outline as it does for someone who tabs.
  await page.keyboard.press("Shift");
  await group.focus();

  await expect(group).toHaveCSS("outline-style", "solid");
  await expect(group).toHaveCSS("outline-width", "2px");
  await expect(group).toHaveCSS("outline-offset", "-2px");
});

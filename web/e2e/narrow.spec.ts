import { expect, masterPassword, test, openApplication, sessionStatus } from "./support/environment";
import { drawnRowFont, drawnRows, outsideTerminal, screenRect, terminalKeyboard } from "./support/terminal";


const hosts = "Host alpha\n\tHostName 198.51.100.10\n\nHost bravo\n\tHostName 198.51.100.11\n";

const sections = [
  { navigation: "Connections", heading: "Connections" },
  { navigation: "Files", heading: "Remote files" },
  { navigation: "Snippets", heading: "Snippets" },
  { navigation: "Config", heading: "Configuration files" },
  { navigation: "Groups", heading: "Groups" },
  { navigation: "Keys", heading: "Keys" },
  { navigation: "Known Hosts", heading: "Known Hosts" },
  { navigation: "Install Key on Server", heading: "Install Key on Server" },
  { navigation: "Ad hoc checks", heading: "Ad hoc checks" },
  { navigation: "Secrets", heading: "The vault" },
  { navigation: "Settings", heading: "Settings" },
  { navigation: "Sync", heading: "Remote sync" },
  { navigation: "History", heading: "History" },
  { navigation: "Terminal", heading: "No console is open" },
] as const;

async function openSectionThroughDrawer(
  page: import("@playwright/test").Page,
  name: string,
  heading = name,
) {
  await expect(sessionStatus(page)).toContainText("Local session active");
  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Primary" })
    .getByRole("link", { name, exact: true })
    .click();
  await expect(page.getByRole("heading", { name: heading, exact: true }).first()).toBeVisible();
}

async function expectNoHorizontalOverflow(page: import("@playwright/test").Page, where: string) {
  const { overflow, culprits } = await page.evaluate(() => {
    const root = document.documentElement;
    const overflow = root.scrollWidth - root.clientWidth;
    if (overflow <= 0) return { overflow, culprits: [] as string[] };

    const limit = root.clientWidth;
    const describe = (element: Element) => {
      const rectangle = element.getBoundingClientRect();
      const identity = [
        element.tagName.toLowerCase(),
        element.id === "" ? "" : `#${element.id}`,
        typeof element.className === "string" && element.className !== ""
          ? `.${element.className.trim().split(/\s+/).join(".")}`
          : "",
      ].join("");
      return `${identity} [${Math.round(rectangle.left)}..${Math.round(rectangle.right)}]`;
    };
    const culprits = [...root.querySelectorAll("*")]
      .filter((element) => {
        const rectangle = element.getBoundingClientRect();
        if (rectangle.width === 0 || rectangle.right <= limit + 0.5) return false;
        return ![...element.children].some(
          (child) => child.getBoundingClientRect().right > limit + 0.5,
        );
      })
      .slice(0, 5)
      .map(describe);
    return { overflow, culprits };
  });
  expect(
    overflow,
    `${where} scrolls sideways at 360px; past the right edge: ${culprits.join(", ") || "nothing measurable"}`,
  ).toBeLessThanOrEqual(0);
}

test("keeps password setup inside 360 pixels without a decorative icon", async ({
  page,
  installation,
}) => {
  await page.goto(installation.url);

  await expect(page.getByText(/cannot be recovered/i)).toBeVisible();
  await expect(page.locator('use[href="#icon-secrets"]')).toHaveCount(0);
  await expectNoHorizontalOverflow(page, "First-run password setup");
  await expectFullyInsideViewport(
    page,
    page.getByRole("button", { name: "Create the vault" }),
    "Create the vault",
  );

  await page.getByLabel("Master password", { exact: true }).fill(masterPassword);
  await page.getByLabel("Confirm master password", { exact: true }).fill(masterPassword);
  await page.getByRole("button", { name: "Create the vault" }).click();
  await expect(sessionStatus(page)).toContainText("Local session active");
  await openSectionThroughDrawer(page, "Secrets", "The vault");
  await page.getByRole("button", { name: "Lock sshc" }).click();

  await expect(page.getByText("Give your master password to open sshc.")).toBeVisible();
  await expect(page.locator('use[href="#icon-secrets"]')).toHaveCount(0);
  await expectNoHorizontalOverflow(page, "Existing-vault password screen");
  await expectFullyInsideViewport(page, page.getByRole("button", { name: "Open" }), "Open sshc");
});

test("draws one separator above the version in the mobile drawer", async ({ page, installation }) => {
  await page.route("**/api/v1/update", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ current: "test", available: false }),
    });
  });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).click();

  const navigation = page.getByRole("navigation", { name: "Primary" });
  const brandMark = navigation.locator("[data-sshc-brand-mark]");
  await expect(brandMark).toBeVisible();
  await expect(brandMark).toHaveAttribute("viewBox", "0 0 512 512");
  await expect(navigation).not.toContainText(">_");
  const version = navigation.getByText(/^Version /);
  await expect(version).toBeVisible();
  const borders = await version.evaluate((node) => {
    const navigation = node.closest("nav");
    let current: HTMLElement | null = node.parentElement;
    let count = 0;
    while (current !== null && current !== navigation) {
      if (Number.parseFloat(getComputedStyle(current).borderTopWidth) > 0) count += 1;
      current = current.parentElement;
    }
    return count;
  });
  expect(borders).toBe(1);
});

test("keeps every section inside 360 pixels", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", hosts);
  await installation.write(
    "sshc/recent-connections.json",
    JSON.stringify({
      schemaVersion: 1,
      entries: [{ alias: "bastion", lastConnectedAt: "2026-08-24T15:30:00Z" }],
    }),
  );
  await page.route("**/api/v1/sync", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: false,
        keyConfigured: false,
        locked: false,
        auto: { enabled: false, phase: "idle" },
        synced: false,
        direction: "both",
      }),
    });
  });
  await openApplication(page, installation);
  await expect(page.getByRole("heading", { name: "Your connections", exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page, "Home");
  await expectNothingCutOff(page, "Home");
  await expectFullyInsideViewport(
    page,
    page.getByRole("list", { name: "Recently used connections" }).getByRole("button", { name: "Connect" }),
    "Recent connection",
  );

  for (const section of sections) {
    await openSectionThroughDrawer(page, section.navigation, section.heading);
    await expectNoHorizontalOverflow(page, section.navigation);
    await expectNothingCutOff(page, section.navigation);
    if (section.navigation === "Known Hosts") {
      await expectFullyInsideViewport(
        page,
        page.getByRole("button", { name: "Delete", exact: true }),
        "Known Hosts delete",
      );
    }
  }
});

async function expectFullyInsideViewport(
  page: import("@playwright/test").Page,
  locator: import("@playwright/test").Locator,
  name: string,
) {
  const box = await locator.boundingBox();
  expect(box, `${name} is not rendered`).not.toBeNull();
  const viewport = page.viewportSize();
  expect(viewport, `${name}: viewport is unavailable`).not.toBeNull();
  expect(box!.x, `${name}: left edge`).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width, `${name}: right edge`).toBeLessThanOrEqual(viewport!.width);
}

async function expectNothingCutOff(page: import("@playwright/test").Page, where: string) {
  const escaped = await page.evaluate(() => {
    const limit = document.documentElement.clientWidth;
    const drawer = document.querySelector("nav");
    const out: string[] = [];
    for (const element of Array.from(
      document.querySelectorAll("input, select, button, a[href], textarea"),
    )) {
      if (drawer?.contains(element) === true) continue;
      if (element.closest("table") !== null) continue;
      const box = element.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) continue;
      if (box.right <= limit + 1) continue;
      const name =
        element.getAttribute("aria-label") ??
        element.textContent?.trim().slice(0, 24) ??
        element.tagName.toLowerCase();
      out.push(
        `${element.tagName.toLowerCase()} "${name}" → ${box.left.toFixed(0)}..${box.right.toFixed(0)} (面は 0..${limit})`,
      );
    }
    return out;
  });
  expect(escaped, `${where}: 面からはみ出した操作がある`).toEqual([]);
}

test("navigates through the drawer and closes it behind itself", async ({ page, installation }) => {
  await openApplication(page, installation);

  const drawer = page.getByRole("navigation", { name: "Primary" });
  const hamburger = page.getByRole("button", { name: "Navigation", exact: true });

  const restingLeft = await drawer.evaluate((element) => element.getBoundingClientRect().left);
  expect(restingLeft).toBeLessThan(0);

  await hamburger.click();
  await expect(hamburger).toHaveAttribute("aria-expanded", "true");
  await drawer.getByRole("link", { name: "Keys", exact: true }).click();

  await expect(page.getByRole("heading", { name: "Keys", exact: true })).toBeVisible();
  await expect(hamburger).toHaveAttribute("aria-expanded", "false");
  await expect
    .poll(() => drawer.evaluate((element) => element.getBoundingClientRect().left))
    .toBeLessThan(0);
});

test("lets the connection detail replace the list and hands back a way out", async ({
  page,
  installation,
}) => {
  await installation.write("conf.d/20-lab.conf", hosts);
  await openApplication(page, installation);
  await openSectionThroughDrawer(page, "Connections");

  const tree = page.getByRole("navigation", { name: "Connections" });
  await expect(tree.getByRole("button", { name: "alpha" })).toBeVisible();

  await tree.getByRole("button", { name: "alpha" }).click();

  await expect(tree).toBeHidden();
  const back = page.getByRole("button", { name: "All connections" });
  await expect(back).toBeVisible();

  await back.click();
  await expect(tree).toBeVisible();
});

test("sends a real control character from the on-screen keys", async ({ page, installation }) => {
  test.skip(
    process.platform === "win32",
    "on-screen keys are a touch affordance; Linux CI covers this path",
  );
  await openApplication(page, installation);

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await nav.getByRole("button", { name: "Local shell" }).click();

  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toBeVisible();
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  await terminalKeyboard(page).focus();
  await page.keyboard.type("sleep 60");
  await page.keyboard.press("Enter");

  const keys = page.getByLabel("On-screen keys");
  await expect(keys).toBeVisible();
  await keys.getByRole("button", { name: "Ctrl", exact: true }).click();
  await terminalKeyboard(page).focus();
  await page.keyboard.type("c");

  await page.keyboard.type('echo the-shell-"came"-back');
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("the-shell-came-back", { timeout: 20_000 });

  const rows = drawnRows(page);
  await terminalKeyboard(page).focus();
  await page.keyboard.type(": zzq");
  await page.keyboard.press("Enter");
  await expect(rows).toContainText("zzq", { timeout: 20_000 });

  await keys.getByRole("button", { name: "↑", exact: true }).click();
  await expect
    .poll(async () => (await rows.innerText()).split("zzq").length - 1, {
      timeout: 20_000,
    })
    .toBeGreaterThanOrEqual(2);
});

test("lays a selectable layer over the terminal, outside the element that blocks it", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await nav.getByRole("button", { name: "Local shell" }).click();

  const rows = drawnRows(page);
  await expect(rows).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type("echo zzq");
  await page.keyboard.press("Enter");
  await expect(rows).toContainText("zzq", { timeout: 20_000 });

  const overlay = page.locator(".sshc-select-overlay");
  await expect(overlay).toHaveCount(1);

  expect(await outsideTerminal(overlay)).toBe(true);
  await expect.poll(async () => (await overlay.textContent()) ?? "").toContain("zzq");

  const screen = await screenRect(page);
  const shape = await page.evaluate((face) => {
    const layer = document.querySelector(".sshc-select-overlay")!.getBoundingClientRect();
    return {
      dx: Math.abs(layer.x - face.x),
      dy: Math.abs(layer.y - face.y),
      dw: Math.abs(layer.width - face.width),
      dh: Math.abs(layer.height - face.height),
    };
  }, screen);
  expect(shape.dx).toBeLessThanOrEqual(1);
  expect(shape.dy).toBeLessThanOrEqual(1);
  expect(shape.dw).toBeLessThanOrEqual(1);
  expect(shape.dh).toBeLessThanOrEqual(1);

  const source = await drawnRowFont(page);
  const metrics = await page.evaluate((drawn) => {
    const layer = getComputedStyle(document.querySelector(".sshc-select-overlay")!);
    return {
      sameFamily: layer.fontFamily === drawn.fontFamily,
      sameSize: layer.fontSize === drawn.fontSize,
      sameSpacing: layer.letterSpacing === drawn.letterSpacing,
      colour: layer.color,
    };
  }, source);
  expect(metrics.sameFamily).toBe(true);
  expect(metrics.sameSize).toBe(true);
  expect(metrics.sameSpacing).toBe(true);
  expect(metrics.colour).toBe("rgba(0, 0, 0, 0)");

  const selected = await page.evaluate(() => {
    const layer = document.querySelector(".sshc-select-overlay")!;
    const range = document.createRange();
    range.selectNodeContents(layer);
    const selection = getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    return selection.toString();
  });
  expect(selected).toContain("zzq");
});

test("asks before closing a live console, in the middle of the screen and not inside the drawer", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("tab", { name: "Terminals" }).click();
  await nav.getByRole("button", { name: "Local shell" }).click();
  await expect(drawnRows(page)).toContainText(/[$#%>]/, { timeout: 20_000 });

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  await nav.getByRole("button", { name: /^Close / }).first().click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();

  const measured = await dialog.evaluate((node) => {
    const box = (node as HTMLElement).getBoundingClientRect();
    return {
      insideDrawer: (node as HTMLElement).closest("nav") !== null,
      left: box.left,
      right: box.right,
      width: box.width,
      viewport: document.documentElement.clientWidth,
    };
  });

  expect(measured.insideDrawer).toBe(false);
  expect(measured.left).toBeGreaterThanOrEqual(0);
  expect(measured.right).toBeLessThanOrEqual(measured.viewport);
  expect(measured.width).toBeGreaterThan(288);

  await expect(dialog.getByRole("button", { name: /Keep|open/i })).toBeVisible();
  await expect(dialog.getByRole("button", { name: /Close/i })).toBeVisible();
});

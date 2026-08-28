import { expect, masterPassword, test, openApplication, sessionStatus } from "./support/environment";
import { drawnRowFont, drawnRows, outsideTerminal, screenRect, terminalKeyboard } from "./support/terminal";


const hosts = "Host alpha\n\tHostName 198.51.100.10\n\nHost bravo\n\tHostName 198.51.100.11\n";

const sections = [
  { navigation: "Connections", heading: "Connections" },
  { navigation: "SFTP", heading: "Remote files" },
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

test("moves from the compact connection browser to detail at 360 pixels", async ({
  page,
  installation,
}) => {
  const entry = await installation.read("config");
  const nas = await installation.read("conf.d/10-home.conf");
  await installation.write(
    "config",
    "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.\n" +
      "# Edit through the UI; lines between these markers are replaced on the next save.\n" +
      "Include connections/home/eu/*.conf\n" +
      "Include connections/home/*.conf\n" +
      "Include groups.sshc.conf\n" +
      "# <<< sshc groups\n" + entry,
  );
  await installation.write("connections/home/nas.conf", nas);
  await installation.write("connections/home/eu/api.conf", "Host eu-api\n\tHostName 198.51.100.21\n\tUser aida\n");
  await installation.write("conf.d/10-home.conf", "");
  await openApplication(page, installation);
  await openSectionThroughDrawer(page, "Connections");
  const browser = page.getByRole("navigation", { name: "Connections" });
  await expect(browser.getByRole("combobox", { name: "Filter by group" })).toBeVisible();
  await expect(browser.getByRole("group", { name: "Arrange connections by" })).toHaveCount(0);
  await expect(browser.getByRole("region", { name: "home/eu group, 1 connections" })).toBeVisible();
  await expectNoHorizontalOverflow(page, "Connections browser");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.waitForTimeout(350);
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-connections-management-mobile-list.png`,
      fullPage: true,
    });
  }

  await browser.getByRole("button", { name: "bastion" }).click();
  await expect(browser).toBeHidden();
  await expect(page.getByRole("heading", { name: "bastion", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "All connections" })).toBeVisible();
  await expectNoHorizontalOverflow(page, "Connection detail");

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.waitForTimeout(350);
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-connections-management-mobile-detail.png`,
      fullPage: true,
    });
  }

  await page.getByRole("button", { name: "All connections" }).click();
  await expect(browser).toBeVisible();
});

test("keeps password setup inside 360 pixels without a decorative icon", async ({
  page,
  installation,
}) => {
  await page.goto(installation.url);

  await expect(page.getByText(/cannot be recovered/i)).toBeVisible();
  await expect(page.locator('use[href="#icon-secrets"]')).toHaveCount(0);
  await expect(page.getByLabel("Theme menu")).toBeVisible();
  await expect(page.getByLabel("Locale menu")).toBeVisible();
  await expectNoHorizontalOverflow(page, "First-run password setup");
  await expectFullyInsideViewport(
    page,
    page.getByRole("button", { name: "Create the vault" }),
    "Create the vault",
  );
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-vault-create-dark-mobile.png`,
      fullPage: true,
    });
    await page.getByLabel("Theme menu").selectOption("light");
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-vault-create-light-mobile.png`,
      fullPage: true,
    });
    await page.getByLabel("Theme menu").selectOption("dark");
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-vault-create-dark-desktop.png`,
      fullPage: true,
    });
    await page.getByLabel("Theme menu").selectOption("light");
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-vault-create-light-desktop.png`,
      fullPage: true,
    });
    await page.getByLabel("Theme menu").selectOption("dark");
    await page.setViewportSize({ width: 360, height: 800 });
  }

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

test("explains an older vault with both schema versions on mobile", async ({ page, installation }) => {
  await page.addInitScript(() => window.localStorage.setItem("sshc.language", "ja"));
  await page.route("**/api/v1/passwords", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        exists: true,
        unlocked: false,
        aliases: [],
        dedicatedKeyPassphrases: [],
      }),
    });
  });
  await page.route("**/api/v1/passwords/unlock", async (route) => {
    await route.fulfill({
      status: 409,
      contentType: "application/problem+json",
      body: JSON.stringify({
        code: "vault_schema_older",
        message: "request rejected",
        currentVersion: 3,
        requiredVersion: 4,
      }),
    });
  });

  await page.goto(installation.url);
  await page.getByLabel("マスターパスワード", { exact: true }).fill(masterPassword);
  await page.getByRole("button", { name: "開く" }).click();

  await expect(page.getByRole("alert")).toContainText(
    "Vault のバージョンが古いです（必要なバージョン: 4、現在: 3）。",
  );
  await expect(page.getByRole("button", { name: "互換性のある Vault を復元" })).toBeVisible();
  await expectNoHorizontalOverflow(page, "古い vault の復旧画面");

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.15.3-vault-version-mobile.png`,
      fullPage: true,
    });
  }
});

test("reports a completed vault migration with both versions on mobile", async ({ page, installation }) => {
  await page.addInitScript(() => window.localStorage.setItem("sshc.language", "ja"));
  await page.goto(installation.url);
  await page.getByLabel("マスターパスワード", { exact: true }).fill(masterPassword);
  await page.getByLabel("マスターパスワード（確認）", { exact: true }).fill(masterPassword);
  await page.getByRole("button", { name: "Vault を作成" }).click();
  await expect(page.getByText(/ローカルセッション有効/).first()).toBeAttached();

  await page.goto(new URL("/secrets", installation.url).toString());
  await page.getByRole("button", { name: "sshc をロック" }).click();
  const unlocked = await page.evaluate(async (passphrase) => {
    const csrf = window.sessionStorage.getItem("sshc.session.csrf") ?? "";
    const response = await fetch("/api/v1/passwords/unlock", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-SSHC-CSRF": csrf },
      body: JSON.stringify({ passphrase }),
    });
    if (!response.ok) throw new Error(`direct unlock failed with ${response.status}`);
    return response.json() as Promise<Record<string, unknown>>;
  }, masterPassword);
  await page.route("**/api/v1/passwords/unlock", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ...unlocked, migratedFromVersion: 4, migratedToVersion: 5 }),
    });
  });
  await page.getByLabel("マスターパスワード", { exact: true }).fill(masterPassword);
  await page.getByRole("button", { name: "開く" }).click();

  await expect(page.getByText("Vault をバージョン 4 から 5 へ安全に更新しました。")).toBeVisible();
  await expectNoHorizontalOverflow(page, "vault migration notice");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.15.3-vault-migration-mobile.png`,
      fullPage: true,
    });
  }
});

test("draws one separator above the version in the mobile drawer", async ({ page, installation }) => {
  await page.route("**/api/v1/update", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ current: "test", available: false }),
    });
  });
  await openApplication(page, installation);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-mobile-home-dark.png`, fullPage: true });
  }
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

  const home = navigation.getByRole("link", { name: "Home", exact: true });
  const terminal = navigation.getByRole("link", { name: "Terminal", exact: true });
  const [homeBox, terminalBox] = await Promise.all([home.boundingBox(), terminal.boundingBox()]);
  expect(homeBox).not.toBeNull();
  expect(terminalBox).not.toBeNull();
  if (homeBox !== null && terminalBox !== null) {
    expect(terminalBox.y + terminalBox.height - homeBox.y).toBeLessThanOrEqual(164);
  }
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-mobile-drawer-dark.png`, fullPage: true });
  }
});

test("uses established product names in the Japanese navigation", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.evaluate(() => window.localStorage.setItem("sshc.language", "ja"));
  await page.reload();
  await expect(sessionStatus(page)).toContainText("ローカルセッション有効");

  const quickConnect = page.locator('section[aria-labelledby="quick-connect-heading"]');
  await expect(quickConnect.locator('[data-sshc-brand-mark="true"]')).toHaveCount(1);
  await expect(quickConnect).not.toContainText(">_");

  await page.getByRole("button", { name: "ナビゲーション", exact: true }).click();
  const navigation = page.getByRole("navigation", { name: "メインナビゲーション" });

  for (const group of ["Main", "Configuration", "Security", "Tools"]) {
    await expect(navigation.locator(`ul[aria-label="${group}"]`)).toBeVisible();
  }
  for (const section of [
    "Home",
    "Connections",
    "Terminal",
    "SFTP",
    "SSH Config",
    "Groups",
    "SSH Keys",
    "Known Hosts",
    "Remote Keys",
    "Diagnostics",
    "Vault",
    "Snippets",
    "Settings",
    "Sync",
    "History",
  ]) {
    await expect(navigation.getByRole("link", { name: section, exact: true })).toBeVisible();
  }
  await expect(navigation.getByRole("tab", { name: "Menu", exact: true })).toHaveAttribute("aria-selected", "true");
  await expect(navigation.getByRole("tab", { name: "Sessions", exact: true })).toBeVisible();
  await expectNoHorizontalOverflow(page, "Japanese navigation");

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-japanese-navigation-mobile.png`,
      fullPage: true,
    });
    await page.setViewportSize({ width: 1280, height: 720 });
    await expect(navigation).toBeVisible();
    await expectNoHorizontalOverflow(page, "Japanese desktop navigation");
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-japanese-navigation-desktop.png`,
      fullPage: true,
    });
  }
});

test("opens quick connection actions above the trigger on mobile", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", hosts);
  await openApplication(page, installation);

  const trigger = page.getByRole("button", { name: "Actions for alpha" });
  await trigger.click();
  const menu = page.getByRole("menu");
  await expect(menu).toBeVisible();

  const [triggerBox, menuBox] = await Promise.all([trigger.boundingBox(), menu.boundingBox()]);
  expect(triggerBox).not.toBeNull();
  expect(menuBox).not.toBeNull();
  if (triggerBox !== null && menuBox !== null) {
    expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(triggerBox.y);
    expect(menuBox.y).toBeGreaterThanOrEqual(0);
  }
  await expect(menu.getByRole("menuitem", { name: "Open connection settings" })).toBeInViewport();
  await expect(menu.getByRole("menuitem", { name: "Connect", exact: true })).toBeInViewport();
});

test("keeps workspace management out of the mobile terminal", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSectionThroughDrawer(page, "Terminal", "No console is open");

  await expect(page.getByRole("button", { name: "Split right" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Split down" })).toHaveCount(0);
  await expect(page.locator("[data-desktop-workspace-controls]")).toBeHidden();
  await expect(page.getByRole("button", { name: "Send command…" })).toBeHidden();
  await expect(page.locator("summary").filter({ hasText: "Saved layouts" })).toBeHidden();
  await expect(page.getByRole("navigation", { name: "Primary" })).toHaveClass(/shadow-none/);
});

test("removes the session status badge from the mobile header", async ({ page, installation }) => {
  await openApplication(page, installation);

  await expect(sessionStatus(page)).toContainText("Local session active");
  await expect(page.locator("[data-session-status-badge]")).toBeHidden();
});

test("keeps mobile navigation and display controls behind header menus", async ({ page, installation }) => {
  await openApplication(page, installation);

  const primaryNavigation = page.getByRole("navigation", { name: "Primary" });
  await expect(primaryNavigation).not.toBeInViewport();
  await expect(page.locator("[data-app-header]")).toHaveCSS("height", "48px");
  await expect(page.locator("[data-app-header]")).toHaveCSS("position", "sticky");

  await page.getByLabel("Display menu").click();
  await expect(page.getByLabel("Theme menu")).toBeVisible();
  await expect(page.getByLabel("Locale menu")).toBeVisible();

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  await expect(primaryNavigation).toBeInViewport();
  await expect(primaryNavigation.getByRole("button", { name: "Search everything" })).toBeVisible();
  await primaryNavigation.getByRole("button", { name: "Search everything" }).click();
  await expect(page.getByRole("dialog", { name: "Search hosts, files, snippets and settings" })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.0-command-palette-mobile.png`, fullPage: true });
  }
});

test("rounds the connection view switch and keeps Config structure aligned while scrolling on mobile", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", hosts);
  await openApplication(page, installation);

  await openSectionThroughDrawer(page, "Connections", "Connections");
  await page.evaluate(() => window.localStorage.setItem("sshc.language", "ja"));
  await page.reload();
  const arrangement = page.getByRole("group", { name: "接続の並べ方" });
  await expect(arrangement).toHaveCSS("border-radius", "8px");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-connections-mobile-rounded-switch.png`, fullPage: true });
  }

  await page.goto(new URL("/config", page.url()).href);
  await expect(page.getByRole("heading", { name: "SSH Config", exact: true }).first()).toBeVisible();

  const metrics = page.locator("[data-config-metrics]");
  await expect(metrics.locator(":scope > div")).toHaveCount(3);
  const boundaries = await metrics.locator(":scope > div").evaluateAll((cells) => cells.slice(0, -1).map((cell, index) => {
    const next = cells[index + 1]!;
    return Number.parseFloat(getComputedStyle(cell).borderRightWidth) +
      Number.parseFloat(getComputedStyle(next).borderLeftWidth);
  }));
  expect(boundaries.every((width) => width >= 1)).toBe(true);

  const alignment = await page.getByRole("button", { name: "conf.d/20-lab.conf" }).evaluate((button) => {
    const row = button.closest("li")?.firstElementChild;
    const icon = row?.querySelector("[data-config-node-icon]");
    const content = icon?.nextElementSibling;
    if (icon === null || icon === undefined || content === null || content === undefined) return null;
    const iconBox = icon.getBoundingClientRect();
    const contentBox = content.getBoundingClientRect();
    return Math.abs((iconBox.top + iconBox.height / 2) - (contentBox.top + contentBox.height / 2));
  });
  expect(alignment).not.toBeNull();
  expect(alignment!).toBeLessThanOrEqual(2);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-config-mobile-metrics.png`, fullPage: true });
  }

  const appHeader = page.locator("[data-app-header]");
  await expect(appHeader).toHaveCSS("position", "sticky");
  const before = await appHeader.boundingBox();
  await metrics.evaluate((element) => {
    let ancestor = element.parentElement;
    while (ancestor !== null && getComputedStyle(ancestor).overflowY !== "auto") ancestor = ancestor.parentElement;
    if (ancestor !== null) ancestor.scrollTop = ancestor.scrollHeight;
  });
  await expect.poll(async () => (await appHeader.boundingBox())?.y).toBe(before?.y);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-config-mobile-sticky.png`, fullPage: true });
  }
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
  const layout = page.getByRole("group", { name: "Connection layout" });
  await expectFullyInsideViewport(
    page,
    layout.getByRole("button", { name: "Panels" }),
    "Panel layout option",
  );
  await expectFullyInsideViewport(
    page,
    layout.getByRole("button", { name: "List" }),
    "List layout option",
  );
  await expectFullyInsideViewport(
    page,
    page.getByRole("button", { name: "Actions for bastion" }),
    "Recent connection actions",
  );
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/sshc-home-quick-access-mobile.png`,
      fullPage: true,
    });
  }

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

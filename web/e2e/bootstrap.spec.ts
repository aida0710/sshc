import { clickAndAwait, expect, openSection, sessionStatus, test, openApplication } from "./support/environment";

test("exchanges the fragment for a session and removes it from the address bar", async ({
  page,
  context,
  installation,
}) => {
  await openApplication(page, installation);

  await expect(page.getByRole("heading", { name: "sshc", level: 1 })).toBeVisible();
  await expect(sessionStatus(page)).toContainText("Local session active");

  expect(await page.evaluate(() => window.location.hash)).toBe("");
  expect(await page.evaluate(() => document.cookie)).toBe("");

  const cookies = await context.cookies();
  const session = cookies.find((cookie) => cookie.name === "sshc_session");
  expect(session).toBeDefined();
  expect(session?.httpOnly).toBe(true);
  expect(session?.sameSite).toBe("Strict");
  expect(session?.secure).toBe(false);
});

test("refuses a replayed bootstrap fragment in a fresh browser context", async ({
  browser,
  installation,
}) => {
  const first = await browser.newContext();
  const firstPage = await first.newPage();
  await firstPage.goto(installation.url);
  await expect(firstPage.getByLabel("Master password", { exact: true })).toBeVisible();
  await first.close();

  const second = await browser.newContext();
  const secondPage = await second.newPage();
  await secondPage.goto(installation.url);
  await expect(secondPage.getByRole("alert")).toContainText(
    "Secure local session could not be started",
  );
  await second.close();
});

test("contacts no origin but its own", async ({ page, installation }) => {
  const requested: string[] = [];
  page.on("request", (request) => requested.push(request.url()));

  await openApplication(page, installation);
  await openSection(page, "Config");
  await expect(page.getByRole("heading", { name: "Include hierarchy" })).toBeVisible();

  const origin = new URL(installation.url).origin;
  const foreign = requested.filter((url) => !url.startsWith(origin) && !url.startsWith("data:"));
  expect(foreign, `these requests left the origin: ${foreign.join(", ")}`).toEqual([]);
});

test("enforces the content security policy in the browser, not only in the header", async ({
  page,
  installation,
}) => {
  const response = await openApplication(page, installation);
  expect(response?.headers()["content-security-policy"]).toBe(
    "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; " +
      "form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
      "img-src 'self' data:; connect-src 'self'; require-trusted-types-for 'script'",
  );

  const inlineRan = await page.evaluate(async () => {
    const marker = "__sshc_inline_marker";
    try {
      const element = document.createElement("script");
      element.textContent = `window.${marker} = true;`;
      document.head.appendChild(element);
    } catch {
      return false;
    }
    await new Promise<void>((done) => requestAnimationFrame(() => done()));
    return Boolean((window as unknown as Record<string, unknown>)[marker]);
  });
  expect(inlineRan, "an inline script executed despite the policy").toBe(false);

  const crossOrigin = await page.evaluate(async () => {
    try {
      await fetch("https://example.invalid/collect", { mode: "no-cors" });
      return "allowed";
    } catch {
      return "blocked";
    }
  });
  expect(crossOrigin).toBe("blocked");
});

test("keeps only the origin-scoped CSRF token in session storage", async ({ page, installation }) => {
  await openApplication(page, installation);
  await expect(sessionStatus(page)).toContainText("Local session active");

  expect(await page.evaluate(() => Object.keys(window.localStorage))).toEqual([]);
  expect(await page.evaluate(() => Object.keys(window.sessionStorage))).toEqual(["sshc.session.csrf"]);
  expect(await page.evaluate(() => window.sessionStorage.getItem("sshc.session.csrf"))).toMatch(
    /^[A-Za-z0-9_-]{43}$/,
  );

  await openSection(page, "Menu");
  await page.getByRole("region", { name: "Menu" }).getByLabel("Language", { exact: true }).selectOption("ja");

  const stored = await page.evaluate(() => ({
    keys: Object.keys(window.localStorage).sort(),
    language: window.localStorage.getItem("sshc.language"),
    session: window.sessionStorage.length,
  }));
  expect(stored.keys).toEqual(["sshc.language"]);
  expect(["en", "ja"]).toContain(stored.language);
  expect(stored.session).toBe(1);
});

test("keeps the chosen appearance, and writes nothing else", async ({ page, installation }) => {
  await openApplication(page, installation);
  await expect(sessionStatus(page)).toContainText("Local session active");

  expect(await page.evaluate(() => Object.keys(window.localStorage))).toEqual([]);

  await openSection(page, "Menu");
  const menu = page.getByRole("region", { name: "Menu" });
  await menu.getByLabel("Theme").selectOption("dark");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

  await menu.getByLabel("Theme").selectOption("system");
  await menu.getByLabel("Language", { exact: true }).selectOption("ja");

  const stored = await page.evaluate(() => ({
    keys: Object.keys(window.localStorage).sort(),
    theme: window.localStorage.getItem("sshc.theme"),
  }));
  expect(stored.keys).toEqual(["sshc.language", "sshc.theme"]);
  expect(stored.theme).toBe("system");
});

test("keeps the chosen language across a reload, and translates the panels", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await expect(sessionStatus(page)).toContainText("Local session active");

  await openSection(page, "Menu");
  await page.getByRole("region", { name: "Menu" }).getByLabel("Language", { exact: true }).selectOption("ja");
  await page.getByRole("navigation", { name: "メインナビゲーション" }).getByRole("link", { name: "Menu", exact: true }).click();
  await expect(page.getByRole("link", { name: "SSH Keysを開く", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "SSH Keysを開く", exact: true }).click();
  await expect(page.getByRole("heading", { name: "SSH Keys", level: 2 })).toBeVisible();
  await expect(page.getByRole("button", { name: "鍵を作成" })).toBeVisible();

  await page.reload();
  await expect(page).toHaveURL(/\/keys$/);
  await expect(page.getByRole("heading", { name: "SSH Keys", level: 2 })).toBeVisible();
  await expect(page.getByRole("button", { name: "鍵を作成" })).toBeVisible();
  expect(await page.evaluate(() => Object.keys(window.localStorage).sort())).toEqual(["sshc.language"]);
});

test("survives a reload", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();

  await page.reload();

  await expect(page).toHaveURL(/\/connections\/servers$/);
  await expect(
    page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }),
  ).toBeVisible();

  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();
  await page.getByLabel("Port", { exact: true }).fill("2255");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  expect(await installation.read("config")).toContain("Port 2255");
});

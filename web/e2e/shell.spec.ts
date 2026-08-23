import { expect, openSection, test, openApplication } from "./support/environment";

const manyHosts = Array.from(
  { length: 40 },
  (_unused, index) => `Host lab-${String(index).padStart(2, "0")}\n\tHostName 198.51.100.${index + 1}\n`,
).join("\n");

test("keeps the header and the primary navigation still while a panel scrolls", async ({
  page,
  installation,
}) => {
  await installation.write("conf.d/20-lab.conf", manyHosts);
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await expect(
    page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "lab-00" }),
  ).toBeVisible();

  const header = page.getByRole("banner");
  const tree = page.getByRole("navigation", { name: "Connections" });
  const resting = await header.boundingBox();
  expect(resting).not.toBeNull();

  const overflow = await tree.evaluate((element) => {
    const scroller = element.parentElement;
    if (scroller === null) return 0;
    return scroller.scrollHeight - scroller.clientHeight;
  });
  expect(overflow, "the fixture is not tall enough to scroll the list").toBeGreaterThan(0);

  const documentOverflow = await page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollHeight - root.clientHeight;
  });
  expect(documentOverflow, "the document scrolls, so the header can leave the viewport").toBe(0);

  const windowOffset = await page.evaluate(() => {
    window.scrollTo(0, 10_000);
    return window.scrollY;
  });
  expect(windowOffset).toBe(0);

  await tree.evaluate((element) => element.parentElement?.scrollTo(0, element.parentElement.scrollHeight));
  expect(await tree.evaluate((element) => element.parentElement?.scrollTop ?? 0)).toBeGreaterThan(0);

  expect(await header.boundingBox()).toEqual(resting);
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeInViewport();
  await expect(page.getByRole("link", { name: "History", exact: true })).toBeInViewport();

  await expect(
    page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "lab-39" }),
  ).toBeInViewport();
});

test("scrolls the primary navigation on its own when the viewport is short", async ({
  page,
  installation,
}) => {
  await page.setViewportSize({ width: 1280, height: 320 });
  await openApplication(page, installation);
  await openSection(page, "Connections");

  const navigation = page.getByRole("navigation", { name: "Primary" });
  const sections = navigation.locator("div.overflow-y-auto");
  const overflow = await sections.evaluate((element) => element.scrollHeight - element.clientHeight);
  expect(overflow, "the section list is not taller than the short viewport").toBeGreaterThan(0);

  const history = page.getByRole("link", { name: "History", exact: true });

  await expect(async () => {
    await history.scrollIntoViewIfNeeded();
    await expect(history).toBeInViewport();
  }).toPass();
  await expect(page.getByRole("banner")).toBeInViewport();
  expect(await sections.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
});

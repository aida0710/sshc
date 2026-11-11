import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, openApplication, openSection, test, type Installation } from "./support/environment";
import { panoramaPNG, portraitPNG, portraitWebM, samplePNG } from "./support/images";
import { openLocalSFTPDirectory } from "./support/sftp";

const screens = [
  { name: "a PC window", width: 1280, height: 800, open: "dblclick", corners: "a dialog's rounded corners", cornerRadius: "12px" },
  { name: "a phone", width: 412, height: 842, open: "click", corners: "square corners, filling the screen,", cornerRadius: "0px" },
] as const;
const pictures = [
  { shape: "tall", file: "portrait.png", bytes: portraitPNG, ratio: 1080 / 2400 },
  { shape: "wide", file: "panorama.png", bytes: panoramaPNG, ratio: 3000 / 300 },
  { shape: "small", file: "small.png", bytes: samplePNG, ratio: 48 / 24 },
];

async function openPreview({ page, installation, screen, file, bytes }: {
  page: Page;
  installation: Installation;
  screen: (typeof screens)[number];
  file: string;
  bytes: Buffer;
}): Promise<Locator> {
  const directory = join(installation.home, "pictures");
  await mkdir(directory, { recursive: true });
  await writeFile(join(directory, file), bytes);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory });
  await page.setViewportSize({ width: screen.width, height: screen.height });
  const row = page.getByTestId("sftp-file-list").getByRole("button", { name: file, exact: true });
  await (screen.open === "click" ? row.click() : row.dblclick());
  return page.getByRole("dialog").first();
}

// shownInArea measures a picture and the preview around it in one frame, once the preview has finished opening.
function shownInArea(picture: Locator) {
  return picture.evaluate(async (element) => {
    await Promise.all(document.getAnimations().map((animation) => animation.finished.catch(() => undefined)));
    const area = element.parentElement?.getBoundingClientRect();
    const shown = element.getBoundingClientRect();
    return { top: shown.top - (area?.top ?? 0), left: shown.left - (area?.left ?? 0), bottom: (area?.bottom ?? 0) - shown.bottom, right: (area?.right ?? 0) - shown.right, ratio: shown.width / shown.height };
  });
}

function expectInside(margins: { top: number; left: number; bottom: number; right: number }) {
  expect(margins.top).toBeGreaterThanOrEqual(-0.5);
  expect(margins.left).toBeGreaterThanOrEqual(-0.5);
  expect(margins.bottom).toBeGreaterThanOrEqual(-0.5);
  expect(margins.right).toBeGreaterThanOrEqual(-0.5);
}

for (const screen of screens) {
  test(`gives the preview ${screen.corners} on ${screen.name}`, async ({ page, installation }) => {
    const preview = await openPreview({ page, installation, screen, file: "small.png", bytes: samplePNG() });

    await expect(preview).toHaveCSS("border-radius", screen.cornerRadius);
  });

  for (const picture of pictures) {
    test(`shows the whole of a ${picture.shape} picture in the preview on ${screen.name}`, async ({ page, installation }) => {
      const preview = await openPreview({ page, installation, screen, file: picture.file, bytes: picture.bytes() });
      const image = preview.locator("[data-preview-zoom-area] img");
      await expect(image).toBeVisible();
      await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(0);

      const visuals = process.env.SSHC_VISUAL_DIR;
      if (visuals !== undefined) await page.screenshot({ path: join(visuals, `preview-${picture.shape}-${screen.width}.png`), animations: "disabled" });
      const margins = await shownInArea(image);
      expectInside(margins);
      expect(margins.ratio).toBeCloseTo(picture.ratio, 1);
    });
  }

  test(`shows the whole of a tall video on ${screen.name}, with every button of the preview free`, async ({ page, installation }) => {
    const preview = await openPreview({ page, installation, screen, file: "portrait.webm", bytes: portraitWebM() });
    const video = preview.locator("video");
    await expect(video).toBeVisible();
    await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.videoWidth)).toBeGreaterThan(0);

    const visuals = process.env.SSHC_VISUAL_DIR;
    if (visuals !== undefined) await page.screenshot({ path: join(visuals, `preview-video-${screen.width}.png`), animations: "disabled" });
    expectInside(await shownInArea(video));

    // The zoom bar floats over the video; a press on the header's buttons must reach them.
    const bar = await preview.getByRole("group", { name: "Zoom" }).boundingBox();
    for (const button of await preview.getByRole("button").all()) {
      const name = await button.getAttribute("aria-label") ?? "";
      const inBar = await button.evaluate((element) => element.closest('[role="group"]')?.getAttribute("aria-label") === "Zoom");
      if (inBar || !await button.isVisible()) continue;
      const box = await button.boundingBox();
      if (bar === null || box === null) continue;
      const apart = box.y + box.height <= bar.y || bar.y + bar.height <= box.y || box.x + box.width <= bar.x || bar.x + bar.width <= box.x;
      expect(apart, `the zoom bar covers ${name}`).toBe(true);
    }
  });
}

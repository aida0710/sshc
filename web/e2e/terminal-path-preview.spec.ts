import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, openSection, test } from "./support/environment";
import { samplePNG } from "./support/images";
import { drawnSpan, terminalKeyboard } from "./support/terminal";

const visualDirectory = process.env.SSHC_VISUAL_DIR;

// The engine's terminal list and stream are replaced so the shell prints one
// known line; the preview itself is read by the real engine from its machine.
async function mockShellPrinting(page: Page, output: string) {
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "image-path" }) });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        sessions: [{
          id: "image-path", kind: "shell", workspaceId: "default", title: "claude", startedAt: "2026-10-23T00:00:00Z",
          state: "connected", problem: "", forwards: [],
        }],
        maxSessions: 50,
      }),
    });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(Buffer.from(output));
  });
}

test("previews the image a local shell printed the path of", async ({ page, installation }) => {
  const imagePath = join(installation.home, "shot.png");
  const laterPath = join(installation.home, "later.png");
  test.skip(!imagePath.startsWith("/"), "terminal paths are detected only when they start with /");
  await writeFile(imagePath, samplePNG());
  await writeFile(laterPath, samplePNG());
  // The line Claude Code prints for an image it sent; the color puts the path
  // in its own span so the test can click it.
  await mockShellPrinting(page, [
    `› [image] \x1b[36m${imagePath}\x1b[0m (1 KB)`,
    `› [image] ${laterPath} (1 KB)`,
  ].join("\r\n"));
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const path = drawnSpan(page, "shot.png");
  await expect(path).toBeVisible();
  const bounds = await path.boundingBox();
  if (bounds === null) throw new Error("the printed path is not visible");
  // xterm's screen receives pointer events above its DOM text rows.
  await page.mouse.click(bounds.x + bounds.width - 4, bounds.y + bounds.height / 2);
  const actions = page.getByRole("dialog", { name: "Terminal link actions" });
  await expect(actions.getByRole("button", { name: "Browse in SFTP" })).toHaveCount(0);
  await actions.getByRole("button", { name: "Preview" }).click();

  const preview = page.getByRole("dialog", { name: imagePath });
  const image = preview.getByRole("img", { name: "shot.png" });
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBe(48);

  // The arrow keys step through the other images the terminal printed.
  await page.keyboard.press("ArrowRight");
  const later = page.getByRole("dialog", { name: laterPath });
  await expect(later.getByRole("img", { name: "later.png" })).toBeVisible();
  await expect(later).toContainText("2 / 2");
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-path-preview.png") });
  }
  await page.keyboard.press("ArrowLeft");
  await expect(preview.getByRole("img", { name: "shot.png" })).toBeVisible();

  await preview.getByRole("button", { name: "Close" }).click();
  await expect(preview).toBeHidden();
});

test("previews an image a real shell printed a path relative to its directory", async ({ page, installation }) => {
  test.skip(process.platform !== "linux", "the engine reads the shell's directory from /proc");
  await mkdir(join(installation.home, "shots"), { recursive: true });
  await writeFile(join(installation.home, "shots", "relative.png"), samplePNG());
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  // The color puts the path in its own span so the test can click it.
  await page.keyboard.type("cd ~ && printf '\\342\\200\\272 [image] \\033[36mshots/relative.png\\033[0m (1 KB)\\n'");
  await page.keyboard.press("Enter");

  const path = drawnSpan(page, "shots/relative.png").last();
  await expect(path).toBeVisible();
  const bounds = await path.boundingBox();
  if (bounds === null) throw new Error("the printed path is not visible");
  await page.mouse.click(bounds.x + bounds.width - 4, bounds.y + bounds.height / 2);
  await page.getByRole("dialog", { name: "Terminal link actions" }).getByRole("button", { name: "Preview" }).click();

  const preview = page.getByRole("dialog", { name: join(installation.home, "shots", "relative.png") });
  await expect(preview.getByRole("img", { name: "relative.png" })).toBeVisible();
});

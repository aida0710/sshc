import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, openSection, test } from "./support/environment";
import { samplePNG } from "./support/images";
import { drawnSpan, terminalKeyboard, terminalScreen, waitForTerminalLayout } from "./support/terminal";

const visualDirectory = process.env.SSHC_VISUAL_DIR;

async function previewPrintedPath(page: Page, text: string) {
  const path = drawnSpan(page, text).last();
  await expect(path).toBeVisible();
  await waitForTerminalLayout(page);
  const bounds = await path.evaluate((element, text) => {
    const node = element.firstChild;
    if (node === null) throw new Error("the printed path has no text");
    const end = (node.textContent ?? "").lastIndexOf(text) + text.length;
    const range = document.createRange();
    range.setStart(node, end - 1);
    range.setEnd(node, end);
    const rectangle = range.getBoundingClientRect();
    return { x: rectangle.x, y: rectangle.y, width: rectangle.width, height: rectangle.height };
  }, text);
  // xterm's screen receives pointer events above its DOM text rows.
  await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  await expect(terminalScreen(page)).toHaveClass(/xterm-cursor-pointer/u);
  await page.mouse.click(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  const actions = page.getByRole("dialog", { name: "Terminal link actions" });
  await expect(actions.getByRole("button", { name: "Browse in SFTP" })).toHaveCount(0);
  await actions.getByRole("button", { name: "Preview" }).click();
}

// The engine's terminal list and stream are replaced so the shell prints one
// known line; the preview itself is read by the real engine from its machine.
async function mockShellPrinting(page: Page, output: string) {
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "image-path" }) });
      return;
    }
    if (path.endsWith("/directory")) {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ path: "/srv/project" }) });
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

  await previewPrintedPath(page, "shot.png");

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

  await previewPrintedPath(page, "shots/relative.png");

  const preview = page.getByRole("dialog", { name: join(installation.home, "shots", "relative.png") });
  await expect(preview.getByRole("img", { name: "relative.png" })).toBeVisible();
});

test("previews ~/ images printed by a real shell away from home and navigates between them", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "this command uses a POSIX shell");
  await mkdir(join(installation.home, "shots"), { recursive: true });
  await mkdir(join(installation.home, "work"), { recursive: true });
  await writeFile(join(installation.home, "shots", "x.png"), samplePNG());
  await writeFile(join(installation.home, "shots", "later.png"), samplePNG());
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  // Quoting preserves the literal ~ in the output. Unquoted echo expands it
  // in the shell and only exercises the existing absolute-path support.
  await page.keyboard.type("cd ~/work && clear && echo '~/shots/x.png' && echo '~/shots/later.png' && echo ~/shots/x.png");
  await page.keyboard.press("Enter");
  await previewPrintedPath(page, "~/shots/x.png");

  const firstPath = join(installation.home, "shots", "x.png");
  const preview = page.getByRole("dialog", { name: firstPath, exact: true });
  const image = preview.getByRole("img", { name: "x.png", exact: true });
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBe(48);
  await expect(preview).toContainText("1 / 2");
  await page.keyboard.press("ArrowRight");
  const later = page.getByRole("dialog", { name: join(installation.home, "shots", "later.png"), exact: true });
  await expect(later.getByRole("img", { name: "later.png" })).toBeVisible();
  await expect(later).toContainText("2 / 2");
  await page.keyboard.press("ArrowLeft");
  await expect(image).toBeVisible();
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-tilde-path-preview.png") });
  }
  await preview.getByRole("button", { name: "Close" }).click();
  await expect(preview).toBeHidden();
  await previewPrintedPath(page, "x.png");
  await expect(image).toBeVisible();
});

test("explains a failed home lookup without treating ~/ as relative to cwd", async ({ page, installation }) => {
  await mockShellPrinting(page, "[image] \x1b[36m~/shots/x.png\x1b[0m (1 KB)");
  await page.route("**/api/v1/sftp/local/entries?path=~", (route) => route.fulfill({
    status: 404, contentType: "application/json", body: JSON.stringify({ code: "sftp_not_found" }),
  }));
  const previews: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.endsWith("/preview")) previews.push(request.url());
  });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await previewPrintedPath(page, "~/shots/x.png");
  await expect(page.getByText("Cannot open this path because the session's home folder could not be read. Try a full path.")).toBeVisible();
  expect(previews).toEqual([]);
});

function shellQuote(text: string): string {
  return "'" + text.replaceAll("'", "'\\''") + "'";
}

test("opens Unicode and spaced image paths from a real shell and uses the same paths for navigation", async ({ page, installation }) => {
  test.skip(process.platform !== "linux", "relative paths use the foreground directory from /proc");
  const folder = join(installation.home, "画像");
  await mkdir(folder);
  await mkdir(join(installation.home, "work"));
  const examples = [
    { name: "画面_unicode.png", printed: "../画像/画面_unicode.png", click: "_unicode.png" },
    { name: "二枚目 double.png", printed: `"${folder}/二枚目 double.png"`, click: "double.png" },
    { name: "三枚目 single.png", printed: `'${folder}/三枚目 single.png'`, click: "single.png" },
    { name: "四枚目 escaped.png", printed: `${folder}/四枚目\\ escaped.png`, click: "escaped.png" },
    { name: "五枚目_home.png", printed: "$HOME/画像/五枚目_home.png", click: "_home.png" },
    { name: "六枚目_braces.png", printed: "${HOME}/画像/六枚目_braces.png", click: "_braces.png" },
    { name: "七枚目 uri.png", printed: pathToFileURL(join(folder, "七枚目 uri.png")).href, click: "uri.png" },
  ];
  for (const example of examples) await writeFile(join(folder, example.name), samplePNG());
  // A script keeps command echo out of the image list. It prints the literal
  // notation, so the browser, not shell expansion, resolves home and escapes.
  const script = examples.map(({ printed }) => `printf '\\033[36m%s\\033[0m\\n' ${shellQuote(printed)}`).join("\n");
  await writeFile(join(installation.home, "print-paths.sh"), script);
  await openApplication(page, installation);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type("cd ~/work && clear && sh ../print-paths.sh");
  await page.keyboard.press("Enter");

  for (const [index, example] of examples.entries()) {
    await previewPrintedPath(page, example.click);
    const preview = page.getByRole("dialog", { name: join(folder, example.name), exact: true });
    const image = preview.getByRole("img", { name: example.name, exact: true });
    await expect(image).toBeVisible();
    await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBe(48);
    await expect(preview).toContainText(`${index + 1} / ${examples.length}`);
    const next = examples[index + 1];
    if (next !== undefined) {
      await page.keyboard.press("ArrowRight");
      await expect(page.getByRole("dialog", { name: join(folder, next.name), exact: true }).getByRole("img")).toBeVisible();
      await page.keyboard.press("ArrowLeft");
      await expect(image).toBeVisible();
    }
    await preview.getByRole("button", { name: "Close" }).click();
    await expect(preview).toBeHidden();
  }
});

test("previews paths hard-wrapped by a real shell from every row and navigates without fragments", async ({ page, installation }) => {
  test.skip(process.platform !== "linux", "relative paths use the foreground directory from /proc");
  // Several short directory components exceed even a desktop terminal width
  // without exceeding the filesystem's per-component name limit.
  const examples = ["absolute", "home", "relative"].map((kind) => {
    const directory = Array<string>(5).fill(`${kind}-long-folder-for-terminal-preview`).join("/");
    const name = kind === "relative" ? "relative image.png" : `${kind}.png`;
    const path = `${directory}/${name}`;
    const printed = kind === "absolute" ? `${installation.home}/${path}` : kind === "home" ? `~/${path}` : `"${path}"`;
    return { directory, name, printed };
  });
  for (const { directory, name } of examples) {
    await mkdir(join(installation.home, directory), { recursive: true });
    await writeFile(join(installation.home, directory, name), samplePNG());
  }
  // Four spaces exercise program-defined indentation rather than Claude Code's two.
  const continuationIndent = "    ";
  const script = [
    "stty size > printed-size.txt",
    "columns=$(awk '{print $2}' printed-size.txt)",
    `printf '%s\\n' ${examples.map(({ printed }) => shellQuote(printed)).join(" ")} |`,
    `awk -v columns="$columns" '{`,
    '  remaining = $0',
    '  printf "\\033[36m"',
    '  while (length(remaining) > columns) {',
    '    printf "%s\\033[K\\r\\033[1B", substr(remaining, 1, columns)',
    `    remaining = "${continuationIndent}" substr(remaining, columns + 1)`,
    '  }',
    '  printf "%s\\033[0m\\r\\n", remaining',
    "}'",
    "printf 'paths-ready\\n'",
  ].join("\n");
  await writeFile(join(installation.home, "print-wrapped-paths.sh"), script);
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type("cd ~ && clear && sh print-wrapped-paths.sh");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("paths-ready");
  const columns = Number((await readFile(join(installation.home, "printed-size.txt"), "utf8")).trim().split(/\s+/)[1]);

  for (const [index, { directory, name, printed }] of examples.entries()) {
    const fragments = [printed.slice(0, columns)];
    for (let offset = columns; offset < printed.length; offset += columns - continuationIndent.length) {
      fragments.push(printed.slice(offset, offset + columns - continuationIndent.length));
    }
    expect(fragments.length).toBeGreaterThan(1);
    for (const fragment of fragments) {
      // The closing quote is intentionally outside the clickable path.
      await previewPrintedPath(page, fragment.replace(/"$/u, ""));
      const preview = page.getByRole("dialog", { name: join(installation.home, directory, name), exact: true });
      const image = preview.getByRole("img", { name, exact: true });
      await expect(image).toBeVisible();
      await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBe(48);
      await expect(preview).toContainText(`${index + 1} / ${examples.length}`);
      if (index < examples.length - 1) {
        await page.keyboard.press("ArrowRight");
        await expect(page.getByRole("dialog", { name: join(installation.home, examples[index + 1]!.directory, examples[index + 1]!.name), exact: true }).getByRole("img")).toBeVisible();
        await page.keyboard.press("ArrowLeft");
        await expect(image).toBeVisible();
      }
      if (visualDirectory !== undefined && index === 0 && fragment === fragments[0]) {
        await page.screenshot({ path: join(visualDirectory, "terminal-hard-wrapped-path-preview.png") });
      }
      await preview.getByRole("button", { name: "Close" }).click();
      await expect(preview).toBeHidden();
    }
  }
});

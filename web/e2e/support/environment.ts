import { test as base, expect, type Page } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";

const binaryPath = resolve(
  process.cwd(),
  "..",
  "bin",
  process.platform === "win32" ? "sshc.exe" : "sshc",
);

function isolatedEnvironment(home: string): NodeJS.ProcessEnv {
  const shared = {
    PATH: process.env.PATH ?? "",
    npm_config_prefix: "/somewhere/desktop",
  };
  if (process.platform !== "win32") {
    return { ...shared, HOME: home };
  }
  const systemRoot = process.env.SystemRoot ?? "C:\\Windows";
  return {
    ...shared,
    HOME: home,
    USERPROFILE: home,
    HOMEDRIVE: home.slice(0, 2),
    HOMEPATH: home.slice(2),
    LOCALAPPDATA: join(home, "AppData", "Local"),
    APPDATA: join(home, "AppData", "Roaming"),
    TEMP: join(home, "Temp"),
    TMP: join(home, "Temp"),
    SystemRoot: systemRoot,
    windir: systemRoot,
    ComSpec: process.env.ComSpec ?? join(systemRoot, "system32", "cmd.exe"),
    PATHEXT: process.env.PATHEXT ?? ".COM;.EXE;.BAT;.CMD",
  };
}

const entryConfig = [
  "# Managed by hand since 2019. Do not reformat.",
  "",
  "Include conf.d/*.conf",
  "",
  "Host bastion",
  "\tHostName=203.0.113.10",
  "\tUser    ops",
  "\tPort 2222",
  "",
  "Host *",
  "\tServerAliveInterval 30",
  "",
].join("\n");

const includedConfig = [
  "Host nas",
  "\tHostName 198.51.100.20",
  "\tUser aida",
  '\tUnknownFutureDirective some "quoted value" 3',
  "",
].join("\n");

const knownHosts =
  "203.0.113.10 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture\n";

export type Installation = {
  home: string;
  url: string;
  read(relative: string): Promise<string>;
  write(relative: string, contents: string): Promise<void>;
};

async function buildHome(): Promise<string> {
  const home = await mkdtemp(join(tmpdir(), "sshc-e2e-"));
  if (!home.startsWith(tmpdir())) {
    throw new Error(
      "the end-to-end home is not inside the temporary directory",
    );
  }
  const root = join(home, ".ssh");
  await mkdir(join(root, "conf.d"), { recursive: true, mode: 0o700 });
  await writeFile(join(root, "config"), entryConfig, { mode: 0o600 });
  await writeFile(join(root, "conf.d", "10-home.conf"), includedConfig, {
    mode: 0o600,
  });
  await writeFile(join(root, "known_hosts"), knownHosts, { mode: 0o600 });
  return home;
}

async function restrictToThisUser(target: string): Promise<void> {
  if (process.platform !== "win32") return;
  const user = process.env.USERNAME ?? "";
  await new Promise<void>((done, fail) => {
    const child = spawn(
      "icacls",
      [
        target,
        "/inheritance:r",
        "/grant",
        "*S-1-5-18:(F)",
        "/grant",
        "*S-1-5-32-544:(F)",
        "/grant",
        `${user}:(F)`,
      ],
      { stdio: "ignore" },
    );
    child.on("error", fail);
    child.on("exit", (code) =>
      code === 0 ? done() : fail(new Error(`icacls ${target} exited ${String(code)}`)),
    );
  });
}

function startBinary(
  home: string,
): Promise<{ child: ChildProcess; url: string }> {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(binaryPath, ["engine"], {
      env: isolatedEnvironment(home),
      stdio: ["ignore", "pipe", "pipe"],
    });
    let exited = false;
    child.on("exit", (code) => {
      exited = true;
      rejectPromise(new Error(`sshc engine exited with ${String(code)}`));
    });

    const deadline = Date.now() + 15_000;
    const ask = () => {
      if (exited) return;
      if (Date.now() > deadline) {
        rejectPromise(new Error("sshc printed no URL within 15s"));
        return;
      }
      const asking = spawn(binaryPath, ["open"], {
        env: isolatedEnvironment(home),
        stdio: ["ignore", "pipe", "ignore"],
      });
      let printed = "";
      asking.stdout?.on("data", (chunk: Buffer) => {
        printed += chunk.toString("utf8");
      });
      asking.on("exit", (code) => {
        const url = printed.trim();
        if (code === 0 && url.startsWith("http://")) {
          resolvePromise({ child, url });
          return;
        }
        setTimeout(ask, 100);
      });
      asking.on("error", () => setTimeout(ask, 100));
    };
    ask();
  });
}

export const windowsShell = process.platform === "win32";

export const shellSays = {
  size: windowsShell
    ? '"$($Host.UI.RawUI.WindowSize.Height)-$($Host.UI.RawUI.WindowSize.Width)"'
    : 'stty size | tr " " "-"',
  redWord: (word: string) =>
    windowsShell
      ? `"$([char]27)[31m${word}$([char]27)[0m"`
      : `printf '\\033[31m${word}\\033[0m\\n'`,
  lateEcho: (text: string) =>
    windowsShell
      ? `Start-Job { Start-Sleep 2; "${text}" } | Out-Null; Start-Sleep 3; Receive-Job * | Out-String`
      : `(sleep 2; echo ${text}) &`,
};

export const test = base.extend<{ installation: Installation }>({
  installation: async ({}, use) => {
    const home = await buildHome();
    const { child, url } = await startBinary(home);
    const installation: Installation = {
      home,
      url,
      async read(relative) {
        return readFile(join(home, ".ssh", relative), "utf8");
      },
      async write(relative, contents) {
        const target = join(home, ".ssh", relative);
        await mkdir(dirname(target), { recursive: true, mode: 0o700 });
        await writeFile(target, contents, { mode: 0o600 });
        if (relative.startsWith("sshc/")) {
          await restrictToThisUser(dirname(target));
          await restrictToThisUser(target);
        }
      },
    };
    await use(installation);
    const exited = new Promise((done) => child.on("exit", done));
    child.stdin?.end();
    const overdue = setTimeout(() => child.kill("SIGKILL"), 10_000);
    await exited;
    clearTimeout(overdue);
    await rm(home, {
      recursive: true,
      force: true,
      maxRetries: 10,
      retryDelay: 100,
    });
  },
});

export const masterPassword = "an end to end master password";

export async function openApplication(
  page: Page,
  installation: { url: string },
) {
  const response = await page.goto(installation.url);
  const confirmation = page.getByLabel("Confirm master password", {
    exact: true,
  });
  await expect(
    page.getByLabel("Master password", { exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Master password", { exact: true })
    .fill(masterPassword);
  if (await confirmation.isVisible()) {
    await confirmation.fill(masterPassword);
    await page.getByRole("button", { name: "Create the vault" }).click();
  } else {
    await page.getByRole("button", { name: "Open" }).click();
  }
  await expect(sessionStatus(page)).toContainText("Local session active");
  return response;
}

export function sessionStatus(page: Page) {
  return page.getByRole("banner").getByRole("status");
}

export async function openSection(page: Page, name: string): Promise<void> {
  await expect(sessionStatus(page)).toContainText("Local session active");
  if (name === "Terminal") {
    // Terminal no longer has a product-menu entry. Most tests reach it by
    // selecting or creating a session; the few empty-state/workspace tests
    // still need to exercise the routable screen directly.
    await page.evaluate(() => {
      window.history.pushState(null, "", "/terminal");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    return;
  }
  const navigation = page.getByRole("navigation", { name: "Primary" });
  if (["Home", "Connections", "SFTP"].includes(name)) {
    await navigation.getByRole("link", { name, exact: true }).click();
    return;
  }
  await navigation.getByRole("link", { name: "Menu", exact: true }).click();
  if (name === "Menu") return;
  await page.getByRole("link", { name: `Open ${name}`, exact: true }).click();
}

export async function openSettingsPage(
  page: Page,
  name: "Engine" | "Terminal" | "Notifications" | "Open connections" | "Master password",
): Promise<void> {
  await expect(sessionStatus(page)).toContainText("Local session active");
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("link", { name: "Menu", exact: true }).click();
  await page.getByRole("link", { name: `Open ${name}`, exact: true }).click();
}

export async function clickAndAwait(
  page: Page,
  buttonName: string,
  pathFragment: string,
  method = "POST",
): Promise<number> {
  const [response] = await Promise.all([
    page.waitForResponse(
      (candidate) =>
        candidate.url().includes(pathFragment) &&
        candidate.request().method() === method,
    ),
    page.getByRole("button", { name: buttonName, exact: true }).click(),
  ]);
  return response.status();
}

export { expect };

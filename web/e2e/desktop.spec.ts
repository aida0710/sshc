import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import {
  _electron as electron,
  expect,
  test,
  type ElectronApplication,
} from "@playwright/test";

type ProcessRow = { pid: number; ppid: number; command: string };

function processRows(): ProcessRow[] {
  const output = execFileSync("ps", ["-eo", "pid=,ppid=,args="], {
    encoding: "utf8",
  });
  return output.split("\n").flatMap((line) => {
    const match = line.match(/^\s*(\d+)\s+(\d+)\s+(.*)$/);
    if (match === null) return [];
    return [
      { pid: Number(match[1]), ppid: Number(match[2]), command: match[3]! },
    ];
  });
}

// ownedEngines は、**この Electron が起こした**エンジンだけを返す。
//
// ブラウザ側のスイートも同じ入口でエンジンを起こす。並行して走る別のワーカーの
// エンジンまで数えると、このテストは他人の後片付けを待つことになる。
function ownedEngines(binary: string, ownerPID: number): ProcessRow[] {
  return processRows().filter(
    ({ command, ppid }) => command === `${binary} engine` && ppid === ownerPID,
  );
}

// engineIsGone は、その pid のエンジンが消えたかを言う。
//
// 終了の確認に親の pid は使えない。Electron が先に消えれば、生き残った
// エンジンは別の親へ引き取られ、**居るのに数えられなくなる**。
function engineIsGone(pid: number): boolean {
  return !processRows().some((row) => row.pid === pid);
}

function testEnvironment(home: string): Record<string, string> {
  const environment: Record<string, string> = {};
  for (const [key, value] of Object.entries(process.env)) {
    if (value !== undefined) environment[key] = value;
  }
  environment.HOME = home;
  environment.XDG_CONFIG_HOME = join(home, ".config");
  return environment;
}

function waitForExit(child: ChildProcess): Promise<number | null> {
  return new Promise((resolveExit, reject) => {
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      reject(new Error("the second Electron instance did not exit within 10s"));
    }, 10_000);
    child.once("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.once("exit", (code) => {
      clearTimeout(timer);
      resolveExit(code);
    });
  });
}

function playwrightElectronLoader(): string {
  const require = createRequire(import.meta.url);
  const packagePath = require.resolve("playwright-core/package.json");
  return join(dirname(packagePath), "lib", "server", "electron", "loader.js");
}

test("Linux desktop owns one direct engine and reuses its window", async () => {
  test.skip(
    process.platform !== "linux",
    "the Linux desktop runtime is checked in Linux CI",
  );
  test.setTimeout(60_000);

  const repository = resolve(process.cwd(), "..");
  const desktopDirectory = join(repository, "desktop");
  const electronExecutable = join(
    desktopDirectory,
    "node_modules",
    "electron",
    "dist",
    "electron",
  );
  const binary = join(repository, "bin", "sshc");
  const home = await mkdtemp(join(tmpdir(), "sshc-electron-e2e-"));
  const environment = testEnvironment(home);
  let desktopApp: ElectronApplication | null = null;
  let enginePID: number | null = null;

  try {
    desktopApp = await electron.launch({
      executablePath: electronExecutable,
      // Playwright only injects this loader when executablePath is omitted. It
      // gates app.whenReady() until both inspector connections are established.
      args: [
        "-r",
        playwrightElectronLoader(),
        "--no-sandbox",
        desktopDirectory,
      ],
      cwd: desktopDirectory,
      env: environment,
      timeout: 30_000,
    });
    const mainPID = desktopApp.process().pid;
    expect(mainPID).toBeDefined();

    const window = await desktopApp.firstWindow();
    await expect(window).toHaveTitle("sshc");
    await expect
      .poll(() => window.url())
      .toMatch(/^http:\/\/127\.0\.0\.1:\d+\//);

    await expect.poll(() => ownedEngines(binary, mainPID!)).toHaveLength(1);
    let engines = ownedEngines(binary, mainPID!);
    enginePID = engines[0]!.pid;

    await desktopApp.evaluate(({ BrowserWindow }) =>
      BrowserWindow.getAllWindows()[0]?.hide(),
    );
    await expect
      .poll(() =>
        desktopApp?.evaluate(({ BrowserWindow }) =>
          BrowserWindow.getAllWindows()[0]?.isVisible(),
        ),
      )
      .toBe(false);

    const second = spawn(
      electronExecutable,
      ["--no-sandbox", desktopDirectory],
      {
        cwd: desktopDirectory,
        env: environment,
        stdio: "ignore",
      },
    );
    expect(await waitForExit(second)).toBe(0);

    await expect
      .poll(() =>
        desktopApp?.evaluate(({ BrowserWindow }) =>
          BrowserWindow.getAllWindows()[0]?.isVisible(),
        ),
      )
      .toBe(true);
    expect(desktopApp.windows()).toHaveLength(1);
    engines = ownedEngines(binary, mainPID!);
    expect(engines).toHaveLength(1);
    expect(engines[0]?.pid).toBe(enginePID);
  } finally {
    if (desktopApp !== null) await desktopApp.close();
    if (enginePID !== null) {
      await expect
        .poll(() => engineIsGone(enginePID!), { timeout: 10_000 })
        .toBe(true);
    }
    await rm(home, { recursive: true, force: true });
  }
});

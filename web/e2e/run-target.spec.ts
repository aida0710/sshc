import type { Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openLocalShell, openSection, test } from "./support/environment";
import { drawnRowFont, markTerminal, terminalKeyboard } from "./support/terminal";

const localVersion = "v0.53.0";
const remoteVersion = "v0.51.0";
const incompatibleHeading = "The sshc on bastion does not work with this machine’s";
const vaultPassword = "bastion vault password";
const sessionsPath = "/api/v1/engines/bastion/terminal/sessions";

// updateOutput is what the stubbed sshc on bastion writes while it updates, in
// the two parts the log answers: the first while it still runs, the rest once
// the test lets it end.
const updateOutput = [
  `sshc: update /home/me/.local/bin/sshc from ${remoteVersion} to ${localVersion} using sshc's install.sh\nsshc: updating ${remoteVersion} to ${localVersion}\n`,
  `sshc: updated to ${localVersion}\nsshc: managed service restarted\n`,
] as const;

// refusal is what the stubbed engine on bastion answers until it is fixed:
// an incompatible version, which updating fixes, a locked Vault, which its
// password unlocks, or an engine that cannot be reached, or whose host SSH
// cannot connect to, until the test says it can. An engine fixed from the start (compatibleVersion)
// refuses from when the test says it does.
type Refusal = "version" | "unreachable" | "connectFailed" | "locked";

async function stubRunTarget(page: Page, { added = true, refusal = "version", compatibleVersion, sessions = [], recentConnections = [] }: { added?: boolean; refusal?: Refusal; compatibleVersion?: string; sessions?: unknown[]; recentConnections?: unknown[] } = {}) {
  let fixed = compatibleVersion !== undefined;
  const actions: unknown[] = [];
  const commandPreviews: unknown[] = [];
  let finishUpdate = () => {};
  const updateFinished = new Promise<void>((resolve) => { finishUpdate = resolve; });
  // What this machine's engine answers the app with when it reads its own SSH configuration.
  const localOverview = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/config/overview" && response.ok())
    .then((response) => response.text());
  await page.route("**/api/v1/health", async (route) => {
    await route.fulfill({ json: { status: "ok", version: localVersion } });
  });
  await page.route("**/api/v1/engines", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({ alias: "bastion" });
      added = true;
      await route.fulfill({ status: 201, json: { alias: "bastion", connected: true } });
    } else {
      await route.fulfill({ json: { engines: added ? [{ alias: "bastion", connected: true }] : [] } });
    }
  });
  await page.route("**/api/v1/engines/bastion/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/control")) {
      actions.push(route.request().postDataJSON());
      await route.fulfill({ status: 202, json: { generation: 1 } });
      return;
    }
    if (path.endsWith("/control/log")) {
      const cursor = Number(new URL(route.request().url()).searchParams.get("cursor") ?? "0");
      const [running, rest] = updateOutput;
      if (cursor < running.length) {
        await route.fulfill({ json: actionLog({ done: false, from: 0, output: running }) });
        return;
      }
      await updateFinished;
      fixed = true;
      await route.fulfill({ json: actionLog({ done: true, from: running.length, output: rest }) });
      return;
    }
    if (refusal === "locked" && path.endsWith("/vault/unlock")) {
      expect(route.request().postDataJSON()).toEqual({ passphrase: vaultPassword });
      fixed = true;
      await route.fulfill({ status: 204 });
      return;
    }
    if (refusal === "locked" && path.endsWith("/status")) {
      await route.fulfill({ json: { alias: "bastion", version: localVersion, vault: true, unlocked: fixed, passwordless: false, sessions: 0 } });
      return;
    }
    if (!fixed) {
      await route.fulfill(refusalResponse(refusal));
      return;
    }
    if (path.endsWith("/status")) {
      await route.fulfill({ json: { alias: "bastion", version: compatibleVersion ?? localVersion, vault: true, unlocked: true, passwordless: true, sessions: 0 } });
    } else if (path.endsWith("/terminal/sessions")) {
      await route.fulfill({ json: { sessions, maxSessions: 50 } });
    } else if (path.endsWith("/terminal/machine")) {
      // Home shows the run target's machine and its connections per day (terminalOverview.ts).
      await route.fulfill({ json: { hostname: "bastion", os: "linux" } });
    } else if (path.endsWith("/terminal/connection-history")) {
      await route.fulfill({ json: { hours: [], recent: [] } });
    } else if (path.includes("/terminal/destinations/")) {
      // bastion cannot resolve the alias, so this machine does not read that session's files.
      await route.fulfill({ status: 422, contentType: "application/problem+json", json: { code: "alias_unresolvable", message: "request rejected" } });
    } else if (path.endsWith("/stream")) {
      // A session shown in the terminal asks for its stream; the test does not need its output.
      await route.fulfill({ json: { streamTicket: "bastion-ticket" } });
    } else if (path.endsWith("/terminal/commands/preview")) {
      // This machine's engine answers this in the real thing: it expands the snippet and asks bastion.
      const request = route.request().postDataJSON() as { targets: { targetId: string; sessionId: string }[] };
      commandPreviews.push(request);
      await route.fulfill({ json: {
        snippetId: "", evidence: "e".repeat(64), reviewEvidence: "r".repeat(64), actionToken: "", actionExpiresAt: "",
        targets: request.targets.map((target) => ({ ...target, alias: "web", title: "web", command: "uptime" })),
      } });
    } else if (path.endsWith("/terminal/restore")) {
      // The engine on bastion has nothing to reopen.
      await route.fulfill({ json: { sessions: [] } });
    } else if (path.endsWith("/terminal/shell-profiles")) {
      // bastion's own shells, which this machine does not have.
      await route.fulfill({ json: { profiles: [{ id: "bastion-fish", label: "fish on bastion", path: "/usr/bin/fish", arguments: [], default: true }] } });
    } else if (path.endsWith("/terminal/workspaces")) {
      await route.fulfill({ json: { workspaces: [
        { id: "default", name: "", hidden: false, builtIn: true },
        { id: "agents", name: "", hidden: true, builtIn: true },
      ], maxWorkspaces: 50 } });
    } else if (path.endsWith("/config/overview")) {
      // bastion's own SSH configuration: this machine's, with one connection under another name.
      await route.fulfill({ contentType: "application/json", body: (await localOverview).replaceAll('"nas"', '"warehouse"') });
    } else if (path.endsWith("/passwords")) {
      // bastion's own Vault, open and empty.
      await route.fulfill({ json: { exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] } });
    } else if (path.endsWith("/credentials")) {
      await route.fulfill({ json: { credentials: [], dedicatedKeyPassphrases: [], keyHostUsageComplete: true } });
    } else if (path.endsWith("/keys")) {
      await route.fulfill({ json: { items: [], unreadable: [], agentDelegations: [], unresolvedReferences: [], agentAvailable: false, agentIdentities: [] } });
    } else if (path.endsWith("/keys/algorithms")) {
      await route.fulfill({ json: { variants: [], source: "bastion" } });
    } else if (path.endsWith("/trash")) {
      await route.fulfill({ json: { entries: [], retentionDays: 30 } });
    } else if (path.endsWith("/vpn")) {
      await route.fulfill({ json: { available: true, profiles: [], checking: false } });
    } else if (path.endsWith("/sftp/transfers")) {
      // bastion's own transfer queue, with nothing in it.
      await route.fulfill({ json: {
        maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: false, largeFileThresholdBytes: 16777216,
        largeFileParallelism: 4, largeFileChunkBytes: 33554432, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0, jobs: [],
      } });
    } else if (path.endsWith("/sftp/local/entries")) {
      // The files of bastion's own machine.
      await route.fulfill({ json: { path: "/home/ops", home: "/home/ops", entries: [
        { name: "only-on-bastion.txt", path: "/home/ops/only-on-bastion.txt", type: "file", size: 12, mode: "-rw-r--r--", modifiedAt: "2026-11-01T00:00:00Z", revision: "r1" },
      ] } });
    } else if (path.endsWith("/known-hosts")) {
      // bastion's own known_hosts.
      await route.fulfill({ json: { path: "/home/ops/.ssh/known_hosts", entries: [
        { line: 1, digest: "d".repeat(64), marker: "", hosts: ["only-bastion-knows.example"], hashed: false, keyType: "ssh-ed25519", fingerprint: "SHA256:bastionbastionbastionbastionbastionbastion12", comment: "" },
      ] } });
    } else if (path.endsWith("/connections/recent")) {
      await route.fulfill({ json: { connections: recentConnections } });
    } else {
      throw new Error(`Unexpected remote engine request: ${path}`);
    }
  });
  return { actions, commandPreviews, fix: () => { fixed = true; }, refuse: () => { fixed = false; }, finishUpdate };
}

// actionLog is the log of the update on bastion from the cursor at from.
function actionLog({ done, from, output }: { done: boolean; from: number; output: string }) {
  const next = from + output.length;
  return {
    generation: 1, action: "update", done, succeeded: done,
    cursor: { requested: from, start: from, next, end: next, truncated: false },
    output,
  };
}

function refusalResponse(refusal: Refusal) {
  switch (refusal) {
    case "version":
      return { status: 409, contentType: "application/problem+json", json: { code: "remote_engine_incompatible", message: "request rejected", detail: remoteVersion } };
    case "locked":
      return { status: 423, contentType: "application/problem+json", json: { code: "remote_engine_vault_locked", message: "request rejected" } };
    case "unreachable":
      return { status: 502, contentType: "application/problem+json", json: { code: "remote_engine_unreachable", message: "request rejected" } };
    case "connectFailed":
      return { status: 502, contentType: "application/problem+json", json: { code: "remote_engine_connect_failed", message: "request rejected", reason: "connection_failed" } };
  }
}

// terminalSession is one session as an engine lists it.
function terminalSession(id: string, title: string, workspaceId = "default") {
  return { id, kind: "shell", workspaceId, title, startedAt: "2026-10-29T00:00:00Z", state: "connected", problem: "", forwards: [] };
}

// stubLocalSessions makes this machine's engine list the sessions given, and add what Home opens to them.
async function stubLocalSessions(page: Page, sessions: ReturnType<typeof terminalSession>[]) {
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 404, json: {} });
    } else if (path === "/api/v1/terminal/sessions" && route.request().method() === "POST") {
      const opened = terminalSession("from-home", "From Home");
      sessions.push(opened);
      await route.fulfill({ status: 201, json: { session: opened, streamTicket: "local-ticket" } });
    } else {
      await route.fulfill({ json: { sessions, maxSessions: 50 } });
    }
  });
}

async function chooseLocalhost(page: Page) {
  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("list", { name: "Engines", exact: true }).getByRole("button", { name: /^localhost/ }).click();
}

async function chooseBastion(page: Page) {
  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("list", { name: "Engines", exact: true }).getByRole("button", { name: /^bastion/ }).click();
}

// screenshotInJapanese saves the screen, wide and narrow, as a person reading Japanese
// sees it, when SSHC_VISUAL_DIR asks for screenshots.
async function screenshotInJapanese(page: Page, name: string) {
  const directory = process.env.SSHC_VISUAL_DIR;
  if (directory === undefined) return;
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "ja");
  await expect(page.getByRole("region").getByRole("button", { name: "localhostに戻す", exact: true })).toBeVisible();
  await page.screenshot({ path: `${directory}/${name}.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  // The sidebar slides away at this width; the screenshot waits until it is gone.
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
  await page.screenshot({ path: `${directory}/${name}-mobile.png` });
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "en");
}

test("updates an incompatible engine from the terminal area and recovers", async ({ page, installation }) => {
  const remote = await stubRunTarget(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: incompatibleHeading, exact: true });
  await expect(panel).toBeVisible();
  await expect(panel.getByText(remoteVersion, { exact: true })).toBeVisible();
  await expect(panel.getByText(localVersion, { exact: true })).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("alert")).toHaveCount(0);
  await screenshotInJapanese(page, "run-target-incompatible");

  const update = panel.getByRole("button", { name: "Update bastion", exact: true });
  await update.click();
  // The first lines show while sshc on bastion still runs.
  const output = panel.getByRole("log", { name: "Output of sshc on that host", exact: true });
  await expect(output).toContainText(`sshc: updating ${remoteVersion} to ${localVersion}`);
  // 待っている間、押したボタンは回る印と「Updating…」に替わる。
  const updating = panel.getByRole("button", { name: "Updating…", exact: true });
  await expect(updating).toHaveAttribute("aria-busy", "true");
  await expect(updating).toBeDisabled();
  await expect(output).not.toContainText("sshc: updated to");
  remote.finishUpdate();
  await expect(panel).toBeHidden();
  expect(remote.actions).toEqual([{ action: "update" }]);
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
});

// Switching the language leaves the terminal screen, which would stop following
// the update, so this screenshot starts in Japanese instead of switching later.
test("shows the update's output in Japanese while sshc on the host runs", async ({ page, installation }) => {
  const directory = process.env.SSHC_VISUAL_DIR;
  test.skip(directory === undefined, "takes screenshots only when SSHC_VISUAL_DIR asks for them");
  const remote = await stubRunTarget(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "ja");
  await page.getByRole("button", { name: /^リモートセッションのsshcエンジンを選ぶ/ }).click();
  await page.getByRole("list", { name: "実行中のsshcエンジン", exact: true }).getByRole("button", { name: "bastion", exact: true }).click();
  const panel = page.getByRole("region", { name: "bastion のsshcとは組み合わせて使えません", exact: true });
  await panel.getByRole("button", { name: "bastion を更新", exact: true }).click();
  await expect(panel.getByRole("log", { name: "接続先の sshc の出力", exact: true })).toContainText("sshc: updating");
  await page.screenshot({ path: `${directory}/run-target-updating.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
  await page.screenshot({ path: `${directory}/run-target-updating-mobile.png` });
  remote.finishUpdate();
  await expect(panel).toBeHidden();
});

test("unlocks a locked vault from the terminal area without a dialog", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "locked" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "The vault on bastion is locked", exact: true });
  const password = panel.getByLabel("Vault password for bastion", { exact: true });
  await expect(password).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await screenshotInJapanese(page, "run-target-locked");

  await password.fill(vaultPassword);
  await panel.getByRole("button", { name: "Unlock", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
});

test("shows an unreachable host in the terminal area and goes away on Reconnect now", async ({ page, installation }) => {
  const remote = await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "Could not connect to bastion", exact: true });
  await expect(panel.getByText("The remote sshc engine cannot be reached. Reconnect, or switch back to localhost.", { exact: true })).toBeVisible();
  await expect(panel.getByText("Tries again 2 s after each attempt ends", { exact: true })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("alert")).toHaveCount(0);
  await panel.getByRole("button", { name: "Details", exact: true }).click();
  await expect(panel.getByText("remote_engine_unreachable", { exact: true })).toBeVisible();
  await screenshotInJapanese(page, "run-target-unreachable");

  // The list is read again every two seconds; right after one reading the
  // next is still far off, so the panel can only go away through Reconnect now.
  await page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath);
  remote.fix();
  await Promise.all([
    page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath && response.ok()),
    panel.getByRole("button", { name: "Reconnect now", exact: true }).click(),
  ]);
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose the remote session engine \(now: bastion\)/ })).toBeVisible();
});

test("sends a command to the sessions of the run target through the run target", async ({ page, installation }) => {
  const session = {
    id: "bastion-web", kind: "ssh", workspaceId: "default", alias: "web", title: "web",
    startedAt: "2026-10-28T01:00:00Z", state: "connected", problem: "", forwards: [],
  };
  const remote = await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [session] });
  await openApplication(page, installation);
  await chooseBastion(page);
  await page.getByRole("button", { name: "Workspace actions for Main", exact: true }).click();
  await page.getByRole("menuitem", { name: "Send a command…", exact: true }).click();
  await page.getByLabel("Command", { exact: true }).fill("uptime");
  await page.getByRole("button", { name: "Preview execution", exact: true }).click();
  await expect(page.getByText("uptime", { exact: true }).last()).toBeVisible();
  expect(remote.commandPreviews).toHaveLength(1);
  expect(remote.commandPreviews[0]).toMatchObject({ command: "uptime", targets: [{ sessionId: "bastion-web" }] });
});

test("switches back to localhost from the terminal area", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "Could not connect to bastion", exact: true });
  await panel.getByRole("button", { name: "Switch back to localhost", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose the remote session engine \(now: localhost\)/ })).toBeVisible();
});

test("keeps this machine's split view while a remote session engine is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await openLocalShell(page);
  const panes = page.locator("[data-split-view-pane]");
  await panes.getByRole("button", { name: "More terminal actions", exact: true }).click();
  await page.getByRole("menuitem", { name: "Show in split view on the right", exact: true }).click();
  await page.getByRole("dialog", { name: "New session on the right" }).getByRole("button", { name: /^Duplicate session/ }).click();
  await expect(panes).toHaveCount(2);
  const splitSessions = await panes.evaluateAll((elements) => elements.map((element) => element.getAttribute("data-split-view-pane")));

  // bastion lists a session of its own, which is shown alone.
  await chooseBastion(page);
  await expect(page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "web", exact: true })).toHaveAttribute("aria-current", "true");
  await expect(panes).toHaveCount(1);
  await expect(panes).toHaveAttribute("data-split-view-pane", "web");
  await chooseLocalhost(page);

  await expect(panes).toHaveCount(2);
  expect(await panes.evaluateAll((elements) => elements.map((element) => element.getAttribute("data-split-view-pane")))).toEqual(splitSessions);
});

test("frames the whole window and names the engine while a remote session is chosen, on every screen", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  const frame = page.locator("[data-remote-session-frame]");
  await expect(frame).toHaveCount(0);

  await chooseBastion(page);
  await expect(frame).toHaveText("Remote session: bastion");
  const viewport = page.viewportSize()!;
  expect(await frame.boundingBox()).toEqual({ x: 0, y: 0, width: viewport.width, height: viewport.height });
  await openSection(page, "Connections");
  await expect(frame).toBeVisible();
  // The frame is only drawn: what is under its name tag can still be pressed.
  await openSection(page, "Home");
  await page.getByRole("button", { name: "Manage connections", exact: true }).click({ trial: true });
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await changeDisplayLanguage(page, "ja");
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/remote-frame-pc.png` });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/remote-frame-phone.png` });
    await page.setViewportSize({ width: 1280, height: 800 });
    await changeDisplayLanguage(page, "en");
  }

  // A phone's header would be under the name tag, so the app starts below it.
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
  const tag = (await frame.locator("span").boundingBox())!;
  const header = (await page.locator("[data-app-header]").boundingBox())!;
  expect(header.y).toBeGreaterThanOrEqual(tag.y + tag.height);
  // ダイアログを開いても名札は手前に出て、ダイアログは名札の下から始まる。スマホではアプリ全体が重なりの基準になり、
  // その中に置いた名札は、body の直下に出るダイアログの下に隠れていた。
  await page.keyboard.press("Control+k");
  const dialogFrame = page.locator(".sshc-modal-frame");
  await expect(dialogFrame).toBeVisible();
  const tagInFront = await frame.evaluate((element: HTMLElement) => {
    const name = element.querySelector("span")!;
    const box = name.getBoundingClientRect();
    // 枠は押す場所を持たない（pointer-events: none）ので、確かめる間だけ当たるようにする。
    element.style.pointerEvents = "auto";
    const front = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    element.style.pointerEvents = "";
    return front === name;
  });
  expect(tagInFront).toBe(true);
  expect((await dialogFrame.boundingBox())!.y).toBeGreaterThanOrEqual(tag.y + tag.height);
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 1280, height: 800 });
  await chooseLocalhost(page);
  await expect(frame).toHaveCount(0);
});

// スマホのようにキーボードが無ければ、コマンド検索から名前で選ぶ。番号は Slack のワークスペースと同じく、
// リモートセッションの一覧の並び（1番目は localhost）。
test("switches the remote session by name from the command search, by its number and to the next one", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  const frame = page.locator("[data-remote-session-frame]");

  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" });
  await palette.getByRole("searchbox").fill("remote session");
  await palette.getByRole("option", { name: /Remote session: bastion/ }).click();
  await expect(frame).toHaveText("Remote session: bastion");

  await page.keyboard.press("Control+1");
  await expect(frame).toHaveCount(0);
  await page.keyboard.press("Control+2");
  await expect(frame).toHaveText("Remote session: bastion");
  // 最後の次は先頭に戻る。
  await page.keyboard.press("Alt+Shift+PageDown");
  await expect(frame).toHaveCount(0);
});

// 枠の色は、その sshc エンジンの接続先（このマシンの Connections）の色。リモートセッションを選んでいる間、Connections は
// リモートの接続先を出すので、一覧の［…］から開くと localhost に戻って、このマシンの接続先を開く。
test("opens the settings of a remote session's connection on this machine from its menu, and frames the window in the colour given there", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);
  const frame = page.locator("[data-remote-session-frame]");
  await expect(frame).toBeVisible();
  const asked: string[] = [];
  page.on("request", (request) => asked.push(new URL(request.url()).pathname));

  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("button", { name: "Actions for engine bastion", exact: true }).click();
  await page.getByRole("button", { name: "Open connection settings (back to localhost)", exact: true }).click();
  await expect(frame).toHaveCount(0);
  await expect(page).toHaveURL(/\/connections\/servers\?.*host=bastion&panel=sshc/);
  await page.getByLabel("Colour", { exact: true }).fill("#2f7fd8");
  const save = page.getByRole("button", { name: "Save", exact: true });
  await save.click();
  await expect(save).toBeHidden();
  expect(asked.filter((path) => path.startsWith("/api/v1/engines/bastion/config"))).toEqual([]);

  await chooseBastion(page);
  await expect(frame).toHaveCSS("border-top-color", "rgb(47, 127, 216)");
});

// 接続先に色が無ければ、さし色。設定（Appearance）で決めた色があれば、そちら。
test("frames the window in the accent colour, or in the colour chosen in Settings, for a connection with no colour", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);
  const frame = page.locator("[data-remote-session-frame]");
  const accent = await page.evaluate(() => {
    const probe = document.createElement("span");
    probe.style.color = "var(--ui-accent)";
    document.body.append(probe);
    const color = getComputedStyle(probe).color;
    probe.remove();
    return color;
  });
  await expect(frame).toHaveCSS("border-top-color", accent);

  await page.goto(new URL("/settings/appearance", page.url()).toString());
  const field = page.getByRole("group", { name: "Remote session frame colour", exact: true });
  await field.getByRole("button", { name: "Custom", exact: true }).click();
  const code = page.getByLabel("Remote session frame colour code", { exact: true });
  await code.fill("#22aa55");
  await code.press("Enter");
  await expect(frame).toHaveCSS("border-top-color", "rgb(34, 170, 85)");
});

test("keeps this machine's terminal as it was while a remote session engine is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Control+Equal");
  await page.keyboard.press("Control+Equal");
  const zoomed = (await drawnRowFont(page)).fontSize;
  expect(zoomed).not.toBe("13px");
  // The mark is on the terminal's own element: a terminal mounted again would not have it.
  const mark = await markTerminal(page, "kept-through-the-switch");

  await chooseBastion(page);
  await expect(page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "web", exact: true })).toHaveAttribute("aria-current", "true");
  // Behind bastion's terminal it is out of sight and out of reach.
  await expect(mark).toBeHidden();
  await chooseLocalhost(page);

  await expect(mark).toBeVisible();
  expect((await drawnRowFont(page)).fontSize).toBe(zoomed);
});

test("keeps what was being typed in a phone's text box while a remote session engine is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  const openNavigation = page.getByRole("button", { name: "Navigation", exact: true });
  await openNavigation.click();
  await openLocalShell(page);
  const field = page.getByRole("textbox", { name: "Text to send to the terminal" });
  await field.fill("half written text");

  await openNavigation.click();
  await chooseBastion(page);
  await expect(page.locator("[data-remote-session-frame]")).toBeVisible();
  await openNavigation.click();
  await chooseLocalhost(page);

  await expect(page.locator("[data-remote-session-frame]")).toHaveCount(0);
  await expect(field).toHaveValue("half written text");
});

test("keeps the run-target switcher usable while the panel is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "locked" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "The vault on bastion is locked", exact: true });
  await expect(panel).toBeVisible();
  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("list", { name: "Engines", exact: true }).getByRole("button", { name: /^localhost/ }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose the remote session engine \(now: localhost\)/ })).toBeVisible();
});

test("keeps the navigation reachable on a phone while the panel is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  await expect(page.getByRole("region", { name: "Could not connect to bastion", exact: true })).toBeVisible();
  // The panel makes the terminal inert, so a phone keeps the app header instead of the
  // terminal's own top bar, and the navigation (with the run-target switcher) stays one tap away.
  await page.setViewportSize({ width: 390, height: 844 });
  const openNavigation = page.getByRole("button", { name: "Navigation", exact: true });
  await expect(openNavigation).toBeVisible();
  await openNavigation.click();
  await expect(page.getByRole("button", { name: /^Choose the remote session engine/ })).toBeVisible();
  // The panel is drawn inside the page, under the open navigation: what is in the middle of the navigation can be pressed.
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "New session", exact: true }).click({ trial: true });
});

test("hides the terminal behind the panel on a phone when the run target stops answering", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const session = {
    id: "bastion-web", kind: "ssh", workspaceId: "default", alias: "web", title: "web",
    startedAt: "2026-10-28T01:00:00Z", state: "connected", problem: "", forwards: [],
  };
  const remote = await stubRunTarget(page, { refusal: "connectFailed", compatibleVersion: localVersion, sessions: [session] });
  await openApplication(page, installation);
  await page.locator("[data-app-header]").getByRole("button", { name: "Navigation", exact: true }).click();
  // Choosing the engine shows the terminal area, with the session shown last on it.
  await chooseBastion(page);
  const composer = page.getByPlaceholder("Type text to send", { exact: true });
  await expect(composer).toBeVisible();

  remote.refuse();
  await expect(page.getByRole("region", { name: "Could not connect to bastion over SSH", exact: true })).toBeVisible();
  // The panel ends above the floating bottom bar, which keeps its margins: what the terminal has at its bottom (the
  // composer, the key bar) must not show around the bar.
  await expect(composer).toBeHidden();
  await page.getByRole("navigation", { name: "Quick navigation", exact: true }).getByRole("link", { name: "Connections", exact: true }).click();
  // Connections opens on its list of servers (connectionRoute).
  await expect(page).toHaveURL(/\/connections\/servers$/);
});

test("selects a newly added engine and shows it is incompatible in the terminal area", async ({ page, installation }) => {
  await stubRunTarget(page, { added: false });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("button", { name: "Add an engine…", exact: true }).click();
  const picker = page.getByRole("dialog", { name: "Add the engine on an SSH host", exact: true });
  await picker.getByRole("button", { name: /^bastion/ }).click();
  const panel = page.getByRole("region", { name: incompatibleHeading, exact: true });
  await expect(panel.getByRole("button", { name: "Update bastion", exact: true })).toBeVisible();
});

// 足せなかった理由はサイドバーの一番下に出て、どの画面でも見えたまま次の操作まで残る。接続先で打つコマンドは
// ［直し方］に畳んである。［もう一度試す］で同じ接続先にもう一度頼み、［×］で消せる。
test("says why an engine could not be added in the sidebar, with the fix folded, a retry and a way to hide it", async ({ page, installation }) => {
  await stubRunTarget(page, { added: false });
  let additions = 0;
  await page.route("**/api/v1/engines", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    additions += 1;
    await route.fulfill({ status: 502, contentType: "application/problem+json", json: { code: "remote_engine_not_running", message: "request rejected" } });
  });
  await openApplication(page, installation);
  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("button", { name: "Add an engine…", exact: true }).click();
  await page.getByRole("dialog", { name: "Add the engine on an SSH host", exact: true }).getByRole("button", { name: /^bastion/ }).click();
  const reason = page.getByRole("alert").filter({ hasText: "The sshc engine is not running" });
  await expect(reason).toBeVisible();
  await expect(reason.getByText("bastion", { exact: true })).toBeVisible();
  const command = reason.getByText("sshc service install", { exact: true });
  await expect(command).toBeHidden();
  await reason.getByText("How to fix", { exact: true }).click();
  await expect(command).toBeVisible();
  await expect(reason.getByRole("button", { name: "Copy sshc open", exact: true })).toBeVisible();

  await reason.getByRole("button", { name: "Try again", exact: true }).click();
  await expect.poll(() => additions).toBe(2);
  await expect(reason).toBeVisible();
  await expect(command).toBeVisible();

  await reason.getByRole("button", { name: "Hide this notice", exact: true }).click();
  await expect(reason).toHaveCount(0);
});

test("uses a compatible engine of another version without a panel", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: remoteVersion });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
  await expect(page.getByRole("region", { name: incompatibleHeading, exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: /^Choose the remote session engine/ }).click();
  await page.getByRole("button", { name: "Actions for engine bastion", exact: true }).click();
  await page.getByRole("button", { name: "Engine information…", exact: true }).click();
  const information = page.getByRole("dialog", { name: "Engine on bastion", exact: true });
  await expect(information.getByRole("definition").filter({ hasText: remoteVersion })).toBeVisible();
  await expect(information.getByText(`this machine runs ${localVersion}`, { exact: true })).toBeVisible();
});

test("says the host is being reached while a session opens, and why it could not be", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion });
  // Opening waits on the SSH connection to bastion, which here never succeeds.
  let answerOpen = () => {};
  const openAnswered = new Promise<void>((resolve) => { answerOpen = resolve; });
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    await openAnswered;
    await route.fulfill(refusalResponse("unreachable"));
  });
  // The SSH connection to the host is what is still missing.
  await page.route("**/api/v1/engines", async (route) => {
    await route.fulfill({ json: { engines: [{ alias: "bastion", connected: false }] } });
  });
  await openApplication(page, installation);
  // Screenshots are taken in Japanese, so the labels follow the language.
  const japanese = process.env.SSHC_VISUAL_DIR !== undefined;
  await chooseBastion(page);
  if (japanese) await changeDisplayLanguage(page, "ja");

  const navigation = page.getByRole("navigation", { name: japanese ? "メインナビゲーション" : "Primary" });
  await navigation.getByRole("button", { name: japanese ? "新しいセッション" : "New session", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: /^Local shell, bastion/ }).click();

  const connecting = page.getByRole("heading", { name: japanese ? "bastionに接続しています" : "Connecting to bastion", exact: true });
  await expect(connecting).toBeVisible();
  await expect(page).toHaveURL(/\/terminal$/);
  if (japanese) {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/run-target-connecting.png` });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/run-target-connecting-mobile.png` });
    await page.setViewportSize({ width: 1280, height: 800 });
  }

  answerOpen();
  await expect(page.getByRole("region", { name: japanese ? "bastion に接続できませんでした" : "Could not connect to bastion", exact: true })).toBeVisible();
  await expect(connecting).toBeHidden();
});

// isOpenOnBastion picks the answer to opening a session on bastion.
function isOpenOnBastion(response: { url: () => string; request: () => { method: () => string } }) {
  return new URL(response.url()).pathname === sessionsPath && response.request().method() === "POST";
}

test("says nothing under this machine when an open on the run target fails after switching back", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion });
  let answerOpen = () => {};
  const openAnswered = new Promise<void>((resolve) => { answerOpen = resolve; });
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    await openAnswered;
    await route.fulfill(refusalResponse("unreachable"));
  });
  await openApplication(page, installation);
  await chooseBastion(page);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: /^Local shell, bastion/ }).click();
  await chooseLocalhost(page);
  await expect(navigation.getByRole("button", { name: "Choose the remote session engine (now: localhost)" })).toBeVisible();

  // The open was asked of bastion, and its refusal arrives once this machine is shown.
  const refused = page.waitForResponse(isOpenOnBastion);
  answerOpen();
  await refused;
  await page.waitForResponse("**/api/v1/terminal/sessions");
  await expect(navigation.getByRole("alert")).toHaveCount(0);
});

test("stops saying a run target cannot be reached once its list answers again", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion });
  // The open does not reach bastion, and the next list does.
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    await route.fulfill(refusalResponse("unreachable"));
  });
  await openApplication(page, installation);
  await chooseBastion(page);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  const refused = page.waitForResponse(isOpenOnBastion);
  await page.getByRole("dialog").getByRole("button", { name: /^Local shell, bastion/ }).click();
  await refused;

  await page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath && response.request().method() === "GET");
  await expect(page.getByRole("region", { name: "Could not connect to bastion", exact: true })).toHaveCount(0);
  await expect(navigation.getByRole("button", { name: "Choose the remote session engine (now: bastion)" })).toBeVisible();
  await expect(navigation.getByRole("alert")).toHaveCount(0);
});

test("asks to reopen the sessions of a run target that restarted between two lists", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion });
  // bastion's sshc restarts so quickly that this machine's engine reaches the
  // new one on its own: no list fails, the next one is empty and names an offer.
  let restarted = false;
  const before = { id: "bastion-shell", kind: "shell", workspaceId: "default", title: "build", startedAt: "2026-10-28T00:00:00Z", state: "connected", problem: "", forwards: [] };
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await route.fulfill({ json: restarted ? { sessions: [], maxSessions: 50, restoreOffer: "after-restart" } : { sessions: [before], maxSessions: 50 } });
  });
  await page.route(`**${sessionsPath}/**`, async (route) => {
    await route.fulfill({ status: 404, contentType: "application/problem+json", json: { code: "terminal_session_not_found", message: "request rejected" } });
  });
  await page.route("**/api/v1/engines/bastion/terminal/restore", async (route) => {
    const offered = restarted ? [{ kind: "shell", workspaceId: "default", pinnedTitle: "build" }] : [];
    await route.fulfill({ json: { sessions: offered } });
  });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const question = page.getByRole("dialog", { name: "Reopen your last sessions?", exact: true });
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByText("build").first()).toBeVisible();
  await expect(question).toHaveCount(0);

  restarted = true;
  await expect(question).toBeVisible();
  await expect(question.getByText(/open on bastion before its sshc stopped/)).toBeVisible();
});

test("offers one local shell, the run target's, for a new session", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion });
  await openApplication(page, installation);
  await chooseBastion(page);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: /^Local shell, bastion/ })).toBeVisible();
  // Which shell it is comes from Settings > Terminal on bastion; the other shells bastion has are not listed.
  await expect(dialog.getByRole("button", { name: /^Local shell ·/ })).toHaveCount(0);
});

test("shows again the session that was open on each run target when switching back to it", async ({ page, installation }) => {
  await stubLocalSessions(page, [terminalSession("build", "Build"), terminalSession("logs", "Logs")]);
  // bastion lists a session in the hidden agents workspace first: the top of the sidebar is web.
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [
    terminalSession("agent-job", "Agent job", "agents"), terminalSession("web", "web"), terminalSession("db", "db"),
  ] });
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const sessionRow = (title: string) => navigation.getByRole("button", { name: title, exact: true });
  await sessionRow("Logs").click();
  await expect(sessionRow("Logs")).toHaveAttribute("aria-current", "true");

  await chooseBastion(page);
  await expect(sessionRow("web")).toHaveAttribute("aria-current", "true");
  await sessionRow("db").click();
  await expect(sessionRow("db")).toHaveAttribute("aria-current", "true");

  await chooseLocalhost(page);
  await expect(sessionRow("Logs")).toHaveAttribute("aria-current", "true");
  await chooseBastion(page);
  await expect(sessionRow("db")).toHaveAttribute("aria-current", "true");
});

test("lists on Home the connections of the chosen engine, not this machine's", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await openSection(page, "Home");
  await expect(page.getByRole("button", { name: /^Connect to nas\./ })).toBeVisible();

  await chooseBastion(page);
  await openSection(page, "Home");
  await expect(page.getByRole("button", { name: /^Connect to warehouse\./ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Connect to nas\./ })).toHaveCount(0);

  await chooseLocalhost(page);
  await openSection(page, "Home");
  await expect(page.getByRole("button", { name: /^Connect to nas\./ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Connect to warehouse\./ })).toHaveCount(0);
});

test("shows in Known Hosts the keys the chosen engine knows, not this machine's", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);
  await openSection(page, "Known Hosts");
  await expect(page.getByText("only-bastion-knows.example", { exact: true })).toBeVisible();

  // The screen is this machine's again once localhost is chosen, without leaving it.
  await chooseLocalhost(page);
  await openSection(page, "Known Hosts");
  await expect(page.getByText("only-bastion-knows.example", { exact: true })).toHaveCount(0);
});

test("lists the chosen engine's connections in Connections and for a new session", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);

  await openSection(page, "Connections");
  const connections = page.getByRole("region", { name: "Connections", exact: true });
  await expect(connections.getByRole("button", { name: "warehouse", exact: true })).toBeVisible();
  await expect(connections.getByRole("button", { name: "nas", exact: true })).toHaveCount(0);

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  const choice = page.getByRole("dialog");
  await expect(choice.getByRole("button", { name: /warehouse/ })).toBeVisible();
  await expect(choice.getByRole("button", { name: /\bnas\b/ })).toHaveCount(0);
});

// 最近使った接続先は、接続した sshc エンジンが覚えている。このマシンはまだどこにも接続していない。
test("offers the chosen engine's recently used connections for a new session and in Connections, not this machine's", async ({ page, installation }) => {
  await stubRunTarget(page, {
    compatibleVersion: localVersion,
    sessions: [terminalSession("web", "web")],
    recentConnections: [{ alias: "warehouse", hostName: "10.0.0.20", user: "ops", port: "22", lastConnectedAt: "2026-11-20T09:30:00Z" }],
  });
  await openApplication(page, installation);
  await chooseBastion(page);

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  const choice = page.getByRole("dialog");
  await expect(choice.getByRole("tab", { name: "Recent", exact: true })).toHaveAttribute("aria-selected", "true");
  await expect(choice.getByRole("button", { name: /warehouse/ })).toBeVisible();
  await page.keyboard.press("Escape");

  await openSection(page, "Connections");
  const connections = page.getByRole("region", { name: "Connections", exact: true });
  await expect(connections.getByText(/^Last connected /)).toHaveCount(1);
});

test("asks the chosen engine, not this machine, for the Vault, the SSH keys and the VPN", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);
  await expect(page.locator("[data-remote-session-frame]")).toBeVisible();
  const asked: string[] = [];
  page.on("request", (request) => asked.push(new URL(request.url()).pathname));

  for (const section of ["Account passwords", "Keys", "VPN"] as const) {
    await openSection(page, section);
  }
  await expect.poll(() => asked).toEqual(expect.arrayContaining([
    "/api/v1/engines/bastion/credentials", "/api/v1/engines/bastion/keys", "/api/v1/engines/bastion/vpn",
  ]));
  expect(asked.filter((path) => ["/api/v1/credentials", "/api/v1/keys", "/api/v1/vpn"].includes(path))).toEqual([]);
});

test("asks about an unsaved connection before the engine changes under it, and keeps it when told to", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion", exact: true }).click();
  await page.getByLabel("Port", { exact: true }).fill("2244");
  await expect(page.getByText("Unsaved changes", { exact: true })).toBeVisible();

  await chooseBastion(page);
  const discardDialog = page.getByRole("dialog", { name: "Discard changes" });
  await expect(discardDialog).toBeVisible();
  await discardDialog.getByRole("button", { name: "Keep editing" }).click();

  // The engine did not change, and what was typed is still there.
  await expect(page.locator("[data-remote-session-frame]")).toHaveCount(0);
  await expect(page.getByLabel("Port", { exact: true })).toHaveValue("2244");
});

test("lists no connection for a new session while the chosen engine's cannot be read", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await chooseBastion(page);
  await expect(page.getByRole("region", { name: "Could not connect to bastion", exact: true })).toBeVisible();

  // This machine's connections are not offered as bastion's.
  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  const choice = page.getByRole("dialog");
  await expect(choice).toBeVisible();
  await expect(choice.getByRole("button", { name: /\bnas\b/ })).toHaveCount(0);
  await expect(choice.getByRole("button", { name: /^SSH.*bastion|^bastion\b/ })).toHaveCount(0);
});

test("shows in SFTP the files of the chosen engine's machine and its connections", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);
  await expect(page.locator("[data-remote-session-frame]")).toBeVisible();
  const asked: string[] = [];
  page.on("request", (request) => asked.push(new URL(request.url()).pathname));

  await openSection(page, "SFTP");
  await expect.poll(() => asked).toContain("/api/v1/engines/bastion/sftp/transfers");
  expect(asked.filter((path) => path.startsWith("/api/v1/sftp/"))).toEqual([]);
});

test("saves a destination typed for a new session in the chosen engine's SSH configuration, where the session opens", async ({ page, installation }) => {
  const sessions = [terminalSession("web", "web")];
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions });
  const saved: unknown[] = [];
  await page.route("**/api/v1/engines/bastion/connections", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    saved.push(route.request().postDataJSON());
    await route.fulfill({ status: 201, json: { transactionId: "tx-1", identity: { path: "config", alias: "10-0-0-9" }, preview: { operation: "connection.create", diffs: [] } } });
  });
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    const opened = terminalSession("saved-destination", "10-0-0-9");
    sessions.push(opened);
    await route.fulfill({ status: 201, json: { session: opened, streamTicket: "bastion-ticket" } });
  });
  const savedHere: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/connections") savedHere.push(request.url());
  });
  await openApplication(page, installation);
  await chooseBastion(page);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByRole("button", { name: "web", exact: true })).toBeVisible();

  await navigation.getByRole("button", { name: "New session", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "New session" });
  await dialog.getByRole("searchbox").fill("deploy@10.0.0.9");
  await dialog.getByRole("button", { name: /^Save as a connection and connect/ }).click();

  await expect.poll(() => saved).toEqual([expect.objectContaining({ hostName: "10.0.0.9", user: "deploy" })]);
  expect(savedHere).toEqual([]);
});

test("opens the folder an SSH session on the chosen engine is in, in that engine's SFTP", async ({ page, installation }) => {
  // warehouse is a connection only bastion has, so this machine's SFTP could not open it.
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [{ ...terminalSession("warehouse-ssh", "warehouse"), kind: "ssh", alias: "warehouse" }] });
  await page.route("**/api/v1/engines/bastion/terminal/sessions/warehouse-ssh/stream", async (route) => {
    await route.fulfill({ status: 201, json: { streamTicket: "warehouse-on-bastion" } });
  });
  // The shell on warehouse says where it is (OSC 7).
  await page.routeWebSocket("**/terminal/stream?**", (socket) => { socket.send(Buffer.from("\x1b]7;file://warehouse/srv/app\x07ops@warehouse:/srv/app$ ")); });
  const sftpRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (path.includes("/sftp/warehouse")) sftpRequests.push(path);
  });
  // bastion's answers for warehouse's files are not what this test looks at.
  await page.route("**/api/v1/engines/bastion/sftp/warehouse/**", async (route) => {
    await route.fulfill({ status: 404, contentType: "application/problem+json", json: { code: "sftp_not_found", message: "request rejected" } });
  });
  await openApplication(page, installation);
  await chooseBastion(page);
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "warehouse", exact: true }).click();
  await page.getByRole("button", { name: "More terminal actions" }).first().click();
  await page.getByRole("menuitem", { name: "Open current directory in SFTP" }).click();

  await expect(page.getByRole("heading", { name: "SFTP", exact: true })).toBeVisible();
  await expect(page.getByText("This terminal link is no longer available.")).toHaveCount(0);
  await expect.poll(() => sftpRequests.some((path) => path.startsWith("/api/v1/engines/bastion/sftp/warehouse"))).toBe(true);
  expect(sftpRequests.filter((path) => !path.startsWith("/api/v1/engines/bastion/"))).toEqual([]);
});

test("names a shell of the chosen engine after that engine, not localhost", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await openApplication(page, installation);
  await chooseBastion(page);
  const row = page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "web", exact: true });
  await expect(row).toContainText("bastion");
  await expect(row).not.toContainText("localhost");
});

test("opens a connection chosen on Home on the run target, which stays chosen", async ({ page, installation }) => {
  const sessions = [terminalSession("web", "web")];
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions });
  const asked: unknown[] = [];
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    asked.push(route.request().postDataJSON());
    const opened = terminalSession("from-home", "From Home");
    sessions.push(opened);
    await route.fulfill({ status: 201, json: { session: opened, streamTicket: "bastion-ticket" } });
  });
  await openApplication(page, installation);
  await chooseBastion(page);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByRole("button", { name: "web", exact: true })).toBeVisible();

  await openSection(page, "Home");
  await page.getByRole("button", { name: /^Connect to bastion\./ }).dblclick();
  await expect(navigation.getByRole("button", { name: "From Home", exact: true })).toHaveAttribute("aria-current", "true");
  expect(asked).toEqual([expect.objectContaining({ kind: "ssh", alias: "bastion" })]);
  await expect(page.getByRole("button", { name: /^Choose the remote session engine \(now: bastion\)/ })).toBeVisible();
});

test("says on a phone why a connection chosen on Home did not open on the run target", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion, sessions: [terminalSession("web", "web")] });
  await page.route(`**${sessionsPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    // bastion's own SSH configuration has no connection of this name.
    await route.fulfill({ status: 422, contentType: "application/problem+json", json: { code: "alias_unresolvable", message: "request rejected" } });
  });
  await openApplication(page, installation);
  await chooseBastion(page);
  await expect(page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "web", exact: true })).toBeVisible();
  await openSection(page, "Home");
  // The sidebar, which keeps the reason, is hidden at this width.
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();

  await page.getByRole("button", { name: /^Connect to bastion\./ }).dblclick();
  await expect(page.getByRole("status").filter({ hasText: "The settings for this connection could not be resolved." })).toBeInViewport();
});

test("says a host is being reached while it does not answer, and going back to this machine does not wait for it", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: localVersion });
  await openApplication(page, installation);
  await openSection(page, "Home");
  const hostAnswers = new Set<string>();
  await page.route("**/api/v1/engines/bastion/terminal/**", () => {
    // A host that does not answer: the request stays open until the browser gives it up.
  });
  await page.route("**/api/v1/engines", async (route) => {
    await route.fulfill({ json: { engines: [{ alias: "bastion", connected: false }] } });
  });
  page.on("requestfailed", (request) => { hostAnswers.add(new URL(request.url()).pathname); });

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "Choose the remote session engine (now: localhost)" }).click();
  await navigation.getByRole("button", { name: /^bastion/ }).click();

  // On any page: the switcher says it, where a page other than the terminal shows nothing of the connection.
  const connecting = navigation.getByRole("button", { name: "Choose the remote session engine (now: bastion, Connecting)" });
  await expect(connecting).toBeVisible();
  await expect(navigation.locator("[data-session-rows-skeleton]")).toBeVisible();

  await page.goto(`${new URL(page.url()).origin}/terminal`);
  await expect(page.getByRole("heading", { name: "Connecting to bastion", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Home" })).toBeHidden();

  // The browser's connections are freed as soon as this machine is chosen, so its requests do not queue behind them.
  const answered = page.waitForResponse("**/api/v1/terminal/sessions");
  await page.getByRole("button", { name: "Switch back to localhost" }).click();
  await answered;
  await expect(navigation.getByRole("button", { name: "Choose the remote session engine (now: localhost)" })).toBeVisible();
  // This machine has no session, so Home takes the terminal's place.
  await expect(page.getByRole("heading", { name: "Home" })).toBeVisible();
  await expect.poll(() => [...hostAnswers]).toContain(sessionsPath);
});

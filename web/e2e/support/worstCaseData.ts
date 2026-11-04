import { execFileSync } from "node:child_process";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import type { Installation } from "./environment";

// The data a screen is least likely to have been drawn for: names far longer than the column, names with nowhere to
// break, Japanese names, and lists long enough to scroll. The layout audit walks every page over it.

// A name a line cannot break in the middle of, as cloud providers give them.
export const longConnectionName = "customer-acme-corporation-production-kubernetes-node-pool-a-0001";
export const unbreakableConnectionName = "a".repeat(96);
export const japaneseConnectionName = "本番環境の東京リージョンにある踏み台サーバーのうちデータベースの移行に使っている第二世代のもの";
export const longGroupName = "customer-acme-corporation-production-environment";
export const japaneseGroupName = "東京リージョンの本番環境";
const longHostName = "ip-10-200-31-187.ap-northeast-1.compute.internal.customer-acme-corporation.example.com";
const longUserName = "ec2-user-with-a-very-long-account-name-for-the-deployment-robot";
const longKeyFileName = "id_ed25519_customer_acme_corporation_production_kubernetes_node_pool_a_0001";

const fleetRegions = ["tokyo", "orchid", "fukuoka", "sapporo", "singapore", "frankfurt", "virginia", "oregon"] as const;
const fleetRoles = ["web", "api", "worker", "cache", "db", "search", "queue", "batch"] as const;

function hostBlock(alias: string, lines: readonly string[]): string {
  return [`Host ${alias}`, ...lines.map((line) => `\t${line}`), ""].join("\n");
}

function groupFolder(group: string): string {
  return `connections/${group}/hosts.conf`;
}

function worstCaseConfig(groups: readonly string[]): string {
  return [
    // The same markers the engine writes around the groups' Include lines.
    "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.",
    "# Edit through the UI; lines between these markers are replaced on the next save.",
    ...groups.map((group) => `Include connections/${group}/*.conf`),
    "Include groups.sshc.conf",
    "# <<< sshc groups",
    "",
    hostBlock("bastion", ["HostName 203.0.113.10", "User ops", "Port 2222"]),
    hostBlock(unbreakableConnectionName, [`HostName ${"h".repeat(120)}.example.com`, `User ${longUserName}`]),
    hostBlock(japaneseConnectionName, ["HostName 198.51.100.20", "User deploy", "ProxyJump bastion"]),
    "Host *\n\tServerAliveInterval 30\n",
  ].join("\n");
}

function worstCaseKnownHosts(): string {
  const lines = Array.from({ length: 60 }, (_, index) =>
    `10.${index % 10}.${Math.floor(index / 10)}.1 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG${String(index).padStart(2, "0")}aXh0dXJla2V5Zml4dHVyZWtleWZpeHR1cmVr`);
  return [
    `${longHostName},${"h".repeat(120)}.example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl`,
    "|1|aGFzaGVkaG9zdG5hbWVmaXh0dXI=|Zml4dHVyZWhhc2hlZHZhbHVlMDA= ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtn",
    ...lines,
    "",
  ].join("\n");
}

// writeWorstCaseHome fills ~/.ssh before the engine is first asked about it.
export async function writeWorstCaseHome(installation: Installation): Promise<void> {
  const fleetGroups = fleetRegions.map((region) => `fleet-${region}`);
  const groups = [longGroupName, japaneseGroupName, ...fleetGroups];
  await installation.write("config", worstCaseConfig(groups));
  await installation.write("groups.sshc.conf", "");
  await installation.write(groupFolder(longGroupName), [
    hostBlock(longConnectionName, [
      `HostName ${longHostName}`,
      `User ${longUserName}`,
      "ProxyJump bastion",
      `IdentityFile ~/.ssh/${longKeyFileName}`,
      "LocalForward 15432 database-primary.internal.customer-acme-corporation.example.com:5432",
      "RemoteForward 9000 127.0.0.1:9000",
      "DynamicForward 1080",
    ]),
    hostBlock(`${longConnectionName}-replica`, [`HostName ${longHostName}`, `User ${longUserName}`, "ProxyJump bastion"]),
    // The same name as in the Tokyo group: Connections warns about it, naming both files.
    hostBlock("web-03", ["HostName 198.51.100.99", "User admin"]),
    // OpenSSH does not know this word, so the file gets a warning.
    "Host typo-host\n\tHostNmae 192.0.2.77\n\tUser deploy\n",
  ].join("\n"));
  await installation.write(groupFolder(japaneseGroupName), [
    hostBlock("web-03", ["HostName 198.51.100.13", "User deploy"]),
    hostBlock("データベース-東京-1", ["HostName 198.51.100.21", "User postgres"]),
  ].join("\n"));
  for (const [regionIndex, region] of fleetRegions.entries()) {
    const blocks = fleetRoles.flatMap((role, roleIndex) =>
      [1, 2].map((number) =>
        hostBlock(`${region}-${role}-${String(number).padStart(2, "0")}`, [`HostName 10.${regionIndex}.${roleIndex}.${number}`, "User deploy", "ProxyJump bastion"])));
    await installation.write(groupFolder(`fleet-${region}`), blocks.join("\n"));
  }
  await installation.write("known_hosts", worstCaseKnownHosts());
  const keyFiles = [longKeyFileName, ...Array.from({ length: 8 }, (_, index) => `id_ed25519_automation_${String(index + 1).padStart(2, "0")}`)];
  for (const file of keyFiles) {
    execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-C", `${file}@ci-runner-with-a-very-long-hostname.example.internal`, "-f", join(installation.home, ".ssh", file)]);
  }
}

// emptyHome is a machine sshc has never been told anything about.
export async function writeEmptyHome(installation: Installation): Promise<void> {
  await installation.write("config", "");
  await installation.write("known_hosts", "");
}

async function callEngine(page: Page, request: { method: string; path: string; body: unknown }): Promise<void> {
  const failure = await page.evaluate(async ({ method, path, body }) => {
    const csrf = window.sessionStorage.getItem("sshc.session.csrf") ?? "";
    const response = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-SSHC-CSRF": csrf },
      body: JSON.stringify(body),
    });
    return response.ok ? "" : `${method} ${path} answered ${response.status}: ${await response.text()}`;
  }, request);
  if (failure !== "") throw new Error(failure);
}

// addWorstCaseRecords fills what the engine keeps behind the signed-in API: the vault's three lists, snippets and
// workspaces.
export async function addWorstCaseRecords(page: Page): Promise<void> {
  const longCredential = "customer-acme-corporation-production-database-administrator";
  for (let number = 1; number <= 14; number++) {
    await callEngine(page, {
      method: "PUT",
      path: `/api/v1/credentials/password/${encodeURIComponent(`${longCredential}-${number}`)}`,
      body: { secret: `fixture-password-${number}` },
    });
  }
  await callEngine(page, { method: "PUT", path: `/api/v1/credentials/password/${encodeURIComponent(`${"x".repeat(120)}`)}`, body: { secret: "fixture-password" } });
  await callEngine(page, { method: "PUT", path: `/api/v1/credentials/password/${encodeURIComponent("本番データベースの管理者アカウントのパスワード（東京リージョン第二世代）")}`, body: { secret: "fixture-password" } });
  await callEngine(page, { method: "PUT", path: `/api/v1/credentials/key_passphrase/${encodeURIComponent(longKeyFileName)}`, body: { secret: "fixture passphrase" } });
  await callEngine(page, { method: "PUT", path: `/api/v1/credentials/totp/${encodeURIComponent(`${longCredential}-root`)}`, body: { secret: "JBSWY3DPEHPK3PXP" } });
  await callEngine(page, { method: "PUT", path: `/api/v1/credentials/totp/${encodeURIComponent("GitHub")}`, body: { secret: "GEZDGNBVGY3TQOJQ" } });
  for (let number = 1; number <= 8; number++) {
    await callEngine(page, {
      method: "POST",
      path: "/api/v1/snippets",
      body: {
        name: `本番のバックアップを取って古いログを消して容量を空ける手順 その${number}`,
        description: "夜間のバッチが止まったときに使う。先に監視を止め、終わったら必ず戻すこと。手順書は運用ページを参照。",
        command: "pg_dump --format=custom --file=/var/backups/postgres/$(date +%Y%m%d-%H%M%S)-{{database}}.dump {{database}} && find /var/log/app -name '*.log' -mtime +{{days}} -delete && df -h /var",
        variables: [{ name: "database", type: "string", required: true, default: "production" }, { name: "days", type: "string", required: true, default: "14" }],
      },
    });
  }
  for (const name of ["障害対応 2026-10-29 レスポンス遅延の調査と顧客への報告とその後の振り返り", longCredential, "短い名前"]) {
    await callEngine(page, { method: "POST", path: "/api/v1/terminal/workspaces", body: { name } });
  }
  await addLongNamedPreset(page);
}

async function addLongNamedPreset(page: Page): Promise<void> {
  const failure = await page.evaluate(async (name) => {
    const headers = { "Content-Type": "application/json", "X-SSHC-CSRF": window.sessionStorage.getItem("sshc.session.csrf") ?? "" };
    const settings = await (await fetch("/api/v1/settings", { credentials: "same-origin", headers })).json() as { presets: { id: string }[] };
    const presets = [...settings.presets, { ...settings.presets[0], id: "long-named-preset", name }];
    const saved = await fetch("/api/v1/settings/presets", { method: "PUT", credentials: "same-origin", headers, body: JSON.stringify({ base: settings.presets, presets }) });
    return saved.ok ? "" : `saving the presets answered ${saved.status}: ${await saved.text()}`;
  }, longGroupName);
  if (failure !== "") throw new Error(failure);
}

const historyStartedAt = "2026-11-03T09:15:42Z";
const historyFinishedAt = "2026-11-03T09:15:43Z";
const longConfigPath = `connections/${longGroupName}/${longKeyFileName}.conf`;

// answerWorstCaseHistory stands in for a long record of changes, each kind of change at least once.
export async function answerWorstCaseHistory(page: Page): Promise<void> {
  const entries = [
    ["connection.update", [longConfigPath, "config"]],
    ["config.restore", ["sshc/secrets/vault.json"]],
    ["config.restore", ["config"]],
    ["key.create", [longKeyFileName, `${longKeyFileName}.pub`]],
    ["settings.save", ["sshc/metadata.json"]],
    ["secret.save", ["sshc/secrets/vault.json"]],
    ["known_hosts.remove", ["known_hosts"]],
    ["group.rename", Array.from({ length: 12 }, (_, index) => `connections/${longGroupName}/host-${index}.conf`)],
    ["unknown.operation", ["config"]],
  ] as const;
  await page.route("**/api/v1/history", (route) => route.fulfill({
    json: {
      entries: Array.from({ length: 30 }, (_, index) => {
        const [operation, paths] = entries[index % entries.length]!;
        return { id: `change-${index}`, operation, status: "completed", startedAt: historyStartedAt, finishedAt: historyFinishedAt, paths, restorable: paths };
      }),
    },
  }));
}

// answerWorstCaseVPN stands in for profiles whose every part is long: Docker decides what the real overview says, and
// a machine running these tests may not have it.
export async function answerWorstCaseVPN(page: Page): Promise<void> {
  const wireguard = (name: string, running: boolean) => ({
    profile: { name, backend: "wireguard", dns: ["10.200.0.2", "10.200.0.3"], wireguard: { servers: [`${longHostName}:51820`] } },
    running,
    relaySocket: "",
    connections: running ? [longConnectionName, `${longConnectionName}-replica`, "bastion"] : [],
    openConnections: running ? 2 : 0,
    ...(running ? { tunnel: { interface: "wg0", address: "10.200.31.187/32", since: historyStartedAt, backend: "wireguard" } } : {}),
  });
  await page.route(/\/api\/v1\/vpn(\?.*)?$/, (route) => route.fulfill({
    json: {
      available: true,
      checking: false,
      profiles: [
        wireguard(longGroupName, true),
        // 48 letters is the longest name a profile can have.
        wireguard("x".repeat(48), false),
        wireguard("本番環境へ入るためのVPN（東京リージョン）", false),
      ],
    },
  }));
}

// answerInterruptedChange makes the engine's overview report a change that stopped half way, as a crash leaves it. It
// changes the answer inside the page, as answerManyMachines does, because the rest of the overview is the real one.
export async function answerInterruptedChange(page: Page): Promise<void> {
  await page.addInitScript((pending) => {
    const realFetch = window.fetch.bind(window);
    window.fetch = async (input, init) => {
      const response = await realFetch(input, init);
      const url = new URL(input instanceof Request ? input.url : String(input), window.location.href);
      if (url.pathname !== "/api/v1/config/overview" || !response.ok) return response;
      const overview = await response.clone().json() as Record<string, unknown>;
      return new Response(JSON.stringify({ ...overview, pending }), { status: response.status, headers: response.headers });
    };
  }, [{
    id: "interrupted-change",
    operation: "connection.update",
    status: "staged",
    startedAt: historyStartedAt,
    committed: 1,
    paths: [longConfigPath, "config"],
    canComplete: true,
  }]);
}

// answerUpdateAvailable says a new release is out, which adds a notice to the navigation and to About.
export async function answerUpdateAvailable(page: Page): Promise<void> {
  await page.route(/\/api\/v1\/update(?:\/check)?$/, (route) => route.fulfill({
    json: {
      current: "v0.60.0",
      latest: "v0.61.0-rc.1+build.20261104.abcdef0123456789",
      available: true,
      canUpdate: false,
      reason: "update_unmanaged",
      checkedAt: historyFinishedAt,
      publishedAt: historyStartedAt,
      releases: [{
        version: "v0.61.0-rc.1+build.20261104.abcdef0123456789",
        body: `# ${"長い見出し".repeat(12)}\n\n${"long-unbroken-word-".repeat(20)}`,
        pageUrl: "https://github.com/aida0710/sshc/releases/tag/v0.61.0",
        publishedAt: historyStartedAt,
      }],
    },
  }));
}

// answerWorstCaseScan stands in for the host a scan would have to reach.
export async function answerWorstCaseScan(page: Page): Promise<void> {
  const fingerprint = "SHA256:U82hDGfzgMUO+I6VhZh4WbmDxN7s/OmPa0g8SBjLzIo";
  const key = "AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl";
  await page.route("**/api/v1/known-hosts/scan", (route) => route.fulfill({
    json: {
      notice: "",
      candidates: [
        { host: longHostName, port: 22, keyType: "ssh-ed25519", key, fingerprint, verified: false },
        { host: longHostName, port: 22, keyType: "ecdsa-sha2-nistp256", key, fingerprint: fingerprint.replace("U82", "V93"), verified: false },
      ],
    },
  }));
}

// answerVPNWithoutDocker is what the overview says on a machine with no Docker, Android among them.
export async function answerVPNWithoutDocker(page: Page): Promise<void> {
  await page.route(/\/api\/v1\/vpn(\?.*)?$/, (route) => route.fulfill({
    json: { available: false, unavailable: "vpn_docker_missing", detail: "docker is not installed or is not on PATH", checking: false, profiles: [] },
  }));
}

// answerConfiguredSync stands in for a bucket whose every name is long and which has synced before: sync needs an
// https endpoint that a machine running these tests does not have.
export async function answerConfiguredSync(page: Page): Promise<void> {
  const summary = { createdAt: historyStartedAt, fileCount: 12_345, sourceBytes: 1_234_567_890, snapshotBytes: 987_654_321 };
  await page.route("**/api/v1/sync", (route) => route.request().method() !== "GET" ? route.fallback() : route.fulfill({
    json: {
      configured: true,
      keyConfigured: true,
      accessKeySuffix: "AMPLE",
      auto: { enabled: true, phase: "idle" },
      locked: false,
      endpoint: `https://${"e".repeat(40)}.r2.cloudflarestorage.customer-acme-corporation.example.com`,
      bucket: `${longGroupName}-sshc-configuration-backup-bucket`,
      region: "ap-northeast-1",
      path: `backups/${longGroupName}/${longKeyFileName}`,
      synced: true,
      direction: "both",
      lastSyncedAt: historyFinishedAt,
      origin: longConnectionName,
      fileCount: 12_345,
      lastOperation: { kind: "push", summary, objectCount: 12_345, uploadedBytes: 987_654_321, completedAt: historyFinishedAt },
    },
  }));
}

// answerManyMachines adds machines with long names to what the engine says about the machines. The engine's own
// machine takes a made-up name: the real one would otherwise show in the pictures this audit keeps. It changes the
// answer inside the page, because the engine refuses a request sent on the page's behalf from outside the browser.
export async function answerManyMachines(page: Page): Promise<void> {
  await page.addInitScript(({ longName, japaneseName }) => {
    const realFetch = window.fetch.bind(window);
    window.fetch = async (input, init) => {
      const response = await realFetch(input, init);
      const url = new URL(input instanceof Request ? input.url : String(input), window.location.href);
      if (url.pathname !== "/api/v1/settings" || (init?.method ?? "GET") !== "GET" || !response.ok) return response;
      const settings = await response.clone().json() as { machines: { id: string; settings: { name?: string } }[] };
      const thisMachine = settings.machines[0]!;
      thisMachine.settings.name = "workstation-01";
      settings.machines.push(
        { ...thisMachine, id: "1".repeat(32), settings: { ...thisMachine.settings, name: longName } },
        { ...thisMachine, id: "2".repeat(32), settings: { ...thisMachine.settings, name: "m".repeat(64) } },
        { ...thisMachine, id: "3".repeat(32), settings: { ...thisMachine.settings, name: japaneseName } },
      );
      return new Response(JSON.stringify(settings), { status: response.status, headers: response.headers });
    };
  }, { longName: longConnectionName, japaneseName: "開発チームの共有マシン（東京オフィスの三階にあるもの）" });
}

// answerAbout hides where the binary really is, which would show the name of the person running the tests in the
// pictures, and gives About the longest values it can have.
export async function answerAbout(page: Page): Promise<void> {
  await page.route("**/api/v1/about", (route) => route.fulfill({
    json: {
      version: "v0.60.0-rc.1+build.20261104.abcdef0123456789",
      revision: "abcdef0123456789abcdef0123456789abcdef0123",
      goVersion: "go1.26.10",
      os: "linux",
      arch: "amd64",
      manager: "unknown",
      executable: `/opt/${"very-long-folder-name/".repeat(6)}sshc`,
    },
  }));
}

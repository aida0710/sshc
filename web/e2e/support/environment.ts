import { test as base, expect, type Page } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";

// binaryPath は試験対象の成果物である。`make e2e` が
// 先にそれをビルドする。バイナリがなければ、dev サーバーへ
// 静かに後退するのではなく大声で失敗する。このスイートの主眼は出荷される成果物だからだ。
const binaryPath = resolve(
  process.cwd(),
  "..",
  "bin",
  process.platform === "win32" ? "sshc.exe" : "sshc",
);

// **Windows は HOME を見ない。** Go の os.UserHomeDir はあちらで USERPROFILE を
// 読むので、HOME だけを渡した子は本物の家を開く——使い捨ての家を用意した意味が
// 消える。加えて、SystemRoot の無い Windows プロセスは DLL の読み込みから
// おかしくなる。internal/acceptance の binary_environment_windows_test.go が
// 同じ問題を同じ形で解いている。
function isolatedEnvironment(home: string): NodeJS.ProcessEnv {
  const shared = {
    PATH: process.env.PATH ?? "",
    // npm_config_prefix は、この常駐プロセスが npm run から起こされた状況で
    // ある。開発中は普通にそうなり、npm は自分の設定を環境に詰めて渡す。
    // 開いた端末がそれを継がないことを terminal.spec が見るので、その状況を
    // ここで作っておく。
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

// フィクスチャの home を書くのはこのファイルだけであり、他の何ものでもない。異
// なる初期状態を必要とする各 spec は `installation.write` を通じてそれを書く。
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
  '\tUnknownFutureDirective some "quoted value" 3',
  "",
].join("\n");

// 構文的には正しいが、まったくの合成物である host key。
// このスイート内の何ものも、それが指すアドレスに接続しない。
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

// restrictToThisUser は、その道を利用者と SYSTEM と Administrators だけに
// 閉じる。**エンジンが要求するのはちょうどこの三つである**
// （internal/platform/windowsacl の isDescriptorRestricted）。
//
// SID で指すのは、SYSTEM と Administrators の表示名が環境で翻訳されるから
// である。名前で書くと、日本語の Windows で無言で外れる。
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
    // HOME は使い捨てのディレクトリであり、PATH が継承されるのは子プロセスが
    // 報告対象になりうる OpenSSH のプログラムを見つけるためだけだ。このスイートの
    // どの spec も、それを起動するルートを引き起こさない。
    //
    // **このスイートは、利用者と同じ手順を踏む。** `sshc engine` を前面で起こし、
    // 入口は `sshc` に刷らせる——engine は入口を出さない（出せばワンタイムの
    // 資格情報が端末にもログにも残る）ので、受け取る道はこれひとつである。
    const child = spawn(binaryPath, ["engine"], {
      env: isolatedEnvironment(home),
      stdio: ["ignore", "pipe", "pipe"],
    });
    let exited = false;
    child.on("exit", (code) => {
      exited = true;
      rejectPromise(new Error(`sshc engine exited with ${String(code)}`));
    });

    // engine が受付を始めるまで、入口は取れない。**待つのは短い間隔で、
    // 上限つきで。** 起こらない起動を無限に待つと、失敗が見えなくなる。
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

// **相手のシェルは OS で違う。** 端末が動いていることを確かめたいのに、
// POSIX の書き方を送れば、PowerShell では構文エラーが返る——確かめたい性質
// ではなく、こちらの方言を検査していることになる。
export const windowsShell = process.platform === "win32";

// shellSays は、同じことを、その OS のシェルの言い方で返す。
export const shellSays = {
  // 端末が自分の大きさを知っているか。POSIX は stty、PowerShell は RawUI。
  size: windowsShell
    ? '"$($Host.UI.RawUI.WindowSize.Height)-$($Host.UI.RawUI.WindowSize.Width)"'
    : 'stty size | tr " " "-"',
  // 少し待ってから書く、を背後で行う。**端末が別の画面の裏でも生きている**
  // ことを、あとから届く一行で言う。
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
        // **エンジンの私的な状態は、閉じていなければ読まれない。** Windows で
        // それを言うのは mode ビットではなく DACL であり、Node が書いたものは
        // 親から継承した ACL を持つ。エンジンは正しくそれを拒む——fixture が
        // 本物と同じ形を作らなければ、確かめているのは「読めない状態」になる。
        if (relative.startsWith("sshc/")) {
          await restrictToThisUser(dirname(target));
          await restrictToThisUser(target);
        }
      },
    };
    await use(installation);
    // **持ち主が手を離すのが、終わり方である。** stdin を閉じるとエンジンは
    // 自分で片付けて終わる。応答しないときのために期限を置くが、そこへ落ちる
    // ことは通常経路ではない。
    const exited = new Promise((done) => child.on("exit", done));
    child.stdin?.end();
    const overdue = setTimeout(() => child.kill("SIGKILL"), 10_000);
    await exited;
    clearTimeout(overdue);
    // **エンジンが終わっても、その子はまだ書いている。** ローカルシェルは
    // 終了の途中で履歴を書き出すので、消しに行った瞬間にディレクトリが
    // 空でないことがある（ENOTEMPTY）。少し待って数回やり直す——
    // 掃除の競争でテストを落とさない。
    await rm(home, {
      recursive: true,
      force: true,
      maxRetries: 10,
      retryDelay: 100,
    });
  },
});

// masterPassword は、すべての spec がアプリケーションを開く
// ときに使うものだ。今やアプリケーション全体がその背後に
// あるため、ロック解除は secret についての試験の一部ではなく起動の一部になっている。
export const masterPassword = "an end to end master password";

// openApplication はナビゲートし、フロントドアを通過する。
//
// 新規インストールの最初の起動は masterPassword の選択を
// 求め、以後の起動はそれの入力を求める。spec がバイナリを
// 再起動しない限り 2 番目の画面には出会わないため、これは
// 最初のケースを扱いつつ、どちらにも対応できるよう書かれている。
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
  // ページがそれをどう扱ったかではなく、ナビゲーション自身の
  // 応答が運んだヘッダーを検証する spec のための、その応答そのもの。
  return response;
}

// sessionStatus はヘッダー自身のステータス行である。
//
// role だけで選ぶのではなく banner にスコープする。パネルは自分自身の
// role="status"要素を持つため、スコープなしのクエリはシェル単体の Vitest
// スイートでは一意でも、組み立てられたアプリケーションでは曖昧になる。
export function sessionStatus(page: Page) {
  return page.getByRole("banner").getByRole("status");
}

// openSection はプライマリナビゲーションを操作する前にまずセッションを待つた
// め、spec がシェルのまだ描画していないパネルをクリックしてしまうことはない。
// 名前は完全一致で照合する。"Keys" は "Install Key on Server" の
// 接頭辞であるため、部分一致では組み立てられたナビゲーションの中で曖昧になる。
export async function openSection(page: Page, name: string): Promise<void> {
  await expect(sessionStatus(page)).toContainText("Local session active");
  await page
    .getByRole("navigation", { name: "Primary" })
    .getByRole("link", { name, exact: true })
    .click();
}

// clickAndAwait はボタンを押し、それが引き起こす API 応答で
// 解決し、その応答の status を返す。
//
// 代わりに見出しを待つのはここでは偽の成功になる。Save
// preview パネルは無条件に見出しを描画するため、見出しを
// 「保存が終わった」ことだと扱う spec は書き込みより前に
// ファイルを読んでしまい、振る舞いではなくタイミングによって合否が決まってしまう。
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

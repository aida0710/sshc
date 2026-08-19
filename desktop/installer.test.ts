import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { engineBinary, managesItsOwnCLI } from "./installer.js";
import { resource } from "./paths.js";

// **resource() を通す。** 走っているのは out/ の中なので、__dirname から
// 直に package.json を指すと存在しない道になる。
const configuration = JSON.parse(
  readFileSync(resource("package.json"), "utf8"),
) as {
  name: string;
  productName: string;
  main: string;
  scripts: Record<string, string>;
  build: {
    files: string[];
    afterPack: string;
    nsis: Record<string, unknown>;
    win: { extraResources: { from: string; to: string }[] };
  };
};
const installerSource = readFileSync(resource("build", "installer.nsh"), "utf8");

// **説明ではなく、書いてあることを見る。** 注釈の中の語で落ちる検査は、
// いずれ注釈を消すことで直される——残すべきものの方が先に消える。
const installerScript = installerSource
  .split("\n")
  .filter((line) => !line.trimStart().startsWith(";"))
  .join("\n");

// **束の中の CLI の場所は、インストーラが PATH へ足す場所と同じでなければ
// ならない。** ずれれば、インストールは成功したのに `sshc` と打っても何も
// 見つからない、という壊れ方をする。
test("the bundled engine sits where each platform expects it", () => {
  assert.strictEqual(
    engineBinary({ platform: "win32", resourcesPath: "C:\\app\\resources" }),
    join("C:\\app\\resources", "cli", "sshc.exe"),
  );
  assert.strictEqual(
    engineBinary({ platform: "darwin", resourcesPath: "/App/Resources" }),
    join("/App/Resources", "sshc"),
  );
  assert.strictEqual(
    engineBinary({ platform: "linux", resourcesPath: "/mnt/resources" }),
    join("/mnt/resources", "sshc"),
  );
});

test("the Windows package copies the CLI into the directory the installer adds to PATH", () => {
  // **${os} を使わない。** electron-builder はこれを "win" に展開するが、
  // Makefile と nativebuild が置くディレクトリは "win32-x64" である。食い違って
  // も electron-builder は黙って飛ばすので、束の中に CLI が入らないまま
  // インストーラが出来上がる——実機に入れて初めて分かった。
  const resources = configuration.build.win.extraResources;
  assert.deepEqual(resources, [
    { from: "resources/win32-${arch}/sshc.exe", to: "cli/sshc.exe" },
  ]);
  assert.ok(
    !JSON.stringify(resources).includes("${os}"),
    "the Windows resource path uses ${os}, which does not expand to win32",
  );
  // installer.nsh が足すのは resources\cli である。上の "to" と同じ場所を
  // 指していることを、綴りの上で確かめる。
  assert.match(installerScript, /!define SSHC_CLI_SUBDIR "resources\\cli"/);
});

// **管理者権限を求めない。** これは配布の方針であり、既定に流されて変わって
// よいものではないので、値を明示したうえでここで固定する。
test("the installer is per-user and never asks to be elevated", () => {
  const nsis = configuration.build.nsis;
  assert.strictEqual(nsis.perMachine, false);
  assert.strictEqual(nsis.allowElevation, false);
  // **黙って入れない。** oneClick の installer は、起動した瞬間に書き込みを
  // 始めて終わる——利用者は、何がどこへ入るのかを見る機会を一度も持たない。
  // 管理者権限を求めないことと、断りなく進めてよいことは別である。
  assert.strictEqual(nsis.oneClick, false);
  // 行き先は動かさない。CLI の場所は installer.nsh と PATH の項目に綴られて
  // おり、選べるようにすると、そのどちらとも食い違いうる。
  assert.strictEqual(nsis.allowToChangeInstallationDirectory, false);
  assert.strictEqual(nsis.include, "build/installer.nsh");
});

// **HKEY_LOCAL_MACHINE にも machine の PATH にも触れない。** 触れば管理者
// 権限が要り、per-user という約束が崩れる。ここが赤くなったら、それは
// 方針が変わったということである。
test("the installer script writes nothing outside this user", () => {
  for (const machineWide of [
    "HKLM",
    "HKEY_LOCAL_MACHINE",
    "SetShellVarContext all",
  ]) {
    assert.ok(
      !installerScript.includes(machineWide),
      `installer.nsh mentions ${machineWide}`,
    );
  }
  // 環境変数を書くのは HKCU\Environment だけである。
  const writes =
    installerScript.match(/Write(RegStr|RegExpandStr) (\S+)/g) ?? [];
  assert.ok(writes.length > 0, "the installer writes nothing at all");
  for (const write of writes) {
    assert.ok(write.endsWith(" HKCU"), `${write} does not write to HKCU`);
  }
});

// **Function を定義しない。** electron-builder は installer と uninstaller を
// 別々にコンパイルし、makensis を警告=エラーで走らせる。`un.` 付きの関数を
// include の時点で置くと、WriteUninstaller を持たない側で 6020 になり、
// **束が一切作れなくなる**——手元で -WX 無しにコンパイルしても気づけない。
test("the installer include defines no functions for either compilation pass", () => {
  const functions = installerScript.match(/^\s*Function\s+\S+/gm) ?? [];
  assert.deepEqual(
    functions,
    [],
    `installer.nsh defines ${functions.join(", ")}; inline the logic instead`,
  );
  assert.ok(
    !installerScript.includes("un."),
    "installer.nsh names an un. symbol, which only exists in the uninstaller pass",
  );
});

// **PATH の項目は、区切りで割って一件ずつ突き合わせる。** 部分文字列で見ると
// `C:\a\sshc` を消すつもりで `C:\a\sshc-tools` の頭を削り、利用者の PATH を
// 壊す。ここでは、その割り方をしていることを構造として見る。
test("the installer matches PATH entries whole, not as substrings", () => {
  // **どのレジスタを使うかは固定しない。** 番号を書き込むと、中身を書き
  // 直しただけで落ち、確かめたい性質とは関係のないところで手が止まる。
  assert.match(installerScript, /\$\{WordFind\} "\$R\d" ";"/);
  // 一件ずつの比較は LogicLib の等値であって、前方一致ではない。取り出した
  // 項目と、足す（消す）項目を、丸ごと突き合わせている。
  assert.match(installerScript, /\$\{If\} \$R\d == \$R\d/);
  assert.match(installerScript, /\$\{If\} \$R\d == \$R\d/);
});

// **自分が書いた登録だけを消す。** 二つの版が入っている機械では、別の場所を
// 指しているものは残っている方のインストールのものである。
test("uninstall removes the launcher value only when it points at this install", () => {
  assert.match(
    installerScript,
    /ReadRegStr \$R\d HKCU "\$\{SSHC_LAUNCHER_KEY\}" "\$\{SSHC_LAUNCHER_VALUE\}"/,
  );
  assert.match(
    installerScript,
    /\$\{If\} \$R\d == "\$INSTDIR\\\$\{PRODUCT_FILENAME\}\.exe"/,
  );
});

// 起動登録の場所は Go 側と対である。**綴りを二箇所に持つので、離れたことに
// 気づける必要がある。**
test("the installer writes the same launcher key the CLI reads", () => {
  assert.match(
    installerScript,
    /!define SSHC_LAUNCHER_KEY "Software\\sshc\\Desktop"/,
  );
  assert.match(installerScript, /!define SSHC_LAUNCHER_VALUE "Executable"/);
});

// **目的ごとに script を分ける。** 一つの `dist` が三つの OS を回そうとすると、
// その OS でしか作れない束を、作れない機械が作ろうとする。
test("each platform has its own dist script and none of them build the others", () => {
  const scripts = configuration.scripts;
  assert.deepEqual(
    Object.keys(scripts)
      .filter((name) => name.startsWith("dist"))
      .sort(),
    ["dist:linux", "dist:mac", "dist:win"],
  );
  // **他の OS の旗を持たないこと。** 綴りの完全一致では、前に付けた
  // `npm run build &&` のような無関係な変更でも赤くなる——見たいのは
  // 「この script が win 以外を作らない」ことだけである。
  const win = scripts["dist:win"] ?? "";
  assert.ok(win.includes("electron-builder --win"), `dist:win does not build win: ${win}`);
  assert.ok(!win.includes("--mac") && !win.includes("--linux"), `dist:win builds another platform: ${win}`);
  assert.ok(!("dist" in scripts), "the combined dist script is still there");

  // **束を作る前に、必ず焼き直す。** TypeScript の出力が out/ に入る以上、
  // 焼かずに electron-builder を呼ぶと、束に入るのは前回の JS である——
  // 直したはずの外殻が入っていない束が、黙って出来上がる。
  for (const platform of ["dist:mac", "dist:linux", "dist:win"]) {
    assert.match(
      scripts[platform] ?? "",
      /^npm run build &&/,
      `${platform} packages without compiling first`,
    );
  }
});

// **Windows では外殻が端末側の入口を作らない。** 安定した場所も PATH も
// インストーラが持っているので、重ねて張ろうとしても失敗を報告するだけになる。
test("only the platforms without an installer manage their own CLI", () => {
  assert.strictEqual(managesItsOwnCLI("win32"), false);
  assert.strictEqual(managesItsOwnCLI("darwin"), true);
  assert.strictEqual(managesItsOwnCLI("linux"), true);
});

// **インストール先の名前は productName ではなく name から来る。** 設計は
// %LOCALAPPDATA%\\Programs\\sshc を約束しており、npm の package 名がそこに
// 現れる。ここが "sshc-desktop" に戻ると、利用者の PATH に入る道も、
// package-smoke が確かめる場所も、README の記載も、まとめてずれる。
test("the install directory is named for the product, not for the npm package", () => {
  assert.strictEqual(configuration.name, "sshc");
  assert.strictEqual(configuration.productName, "sshc");
});

// ships は、electron-builder の files の綴りで、その道が束に入るかを答える。
//
// 使っているのは `*`（`/` を跨がない任意）と、先頭の `!`（打ち消し）だけである。
// **files に書いてよい綴りをこの二つに限っているから、ここで判定できる。**
function ships(path: string): boolean {
  const matches = (pattern: string): boolean =>
    new RegExp(`^${pattern.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, "[^/]*")}$`).test(path);
  const positive = configuration.build.files.filter((p) => !p.startsWith("!"));
  const negative = configuration.build.files.filter((p) => p.startsWith("!"));
  return positive.some(matches) && !negative.some((p) => matches(p.slice(1)));
}

// requiredModules は、main.ts から辿れる自前の module をすべて返す。
//
// **一覧を手で並べない。** かつてここは 4 つを名指しで並べており、その後に
// 増えた entrance・tray・reopen・paths は誰にも数えられていなかった。束から
// 漏れた module は、**起動して初めて** `Cannot find module` で分かる——CI では
// 一度も起こらない失敗である。辿って求めれば、次に増えるものも自動で入る。
function requiredModules(): string[] {
  const found = new Set<string>();
  const walk = (name: string): void => {
    if (found.has(name)) return;
    found.add(name);
    const source = readFileSync(resource(`${name}.ts`), "utf8");
    for (const reference of source.matchAll(/from "\.\/([\w-]+)\.js"/g)) {
      walk(reference[1] as string);
    }
  };
  walk("main");
  // preload は誰も import しない。**main が実行時に道として渡すだけ**なので、
  // 辿っては見つからない——ここだけは名指しで足す。
  found.add("preload");
  return [...found];
}

// 束に入るファイルの一覧から漏れると、実行時に require が失敗する。
test("the packaged files include every module the shell loads", () => {
  const modules = requiredModules();
  assert.ok(modules.length >= 9, `only ${modules.length} modules were traced`);
  for (const module of modules) {
    assert.ok(ships(`out/${module}.js`), `out/${module}.js is not packaged`);
  }
});

// **束の入口は、束に入るものでなければならない。**
test("the entry point named by package.json is itself packaged", () => {
  assert.strictEqual(configuration.main, "out/main.js");
  assert.ok(ships(configuration.main), "the main entry point is not packaged");
});

// **テストは出荷しない。** out/ をまとめて入れる綴りにした以上、打ち消しが
// 効いていることを確かめないと、一時ディレクトリを作る道具が束の中へ入る。
test("the tests and the build-time hook stay out of the bundle", () => {
  for (const excluded of [
    "out/installer.test.js",
    "out/install-cli.test.js",
    "out/paths.test.js",
    "out/adhoc.js",
  ]) {
    assert.ok(!ships(excluded), `${excluded} is packaged`);
  }
  // afterPack の hook は束の外から読まれる。**在ることは要るが、入ることは要らない。**
  assert.strictEqual(configuration.build.afterPack, "./out/adhoc.js");
});

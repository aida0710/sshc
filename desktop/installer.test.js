"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { engineBinary, managesItsOwnCLI } = require("./installer");

const configuration = JSON.parse(
  readFileSync(join(__dirname, "package.json"), "utf8"),
);
const installerSource = readFileSync(
  join(__dirname, "build", "installer.nsh"),
  "utf8",
);

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
  const resources = configuration.build.win.extraResources;
  assert.deepEqual(resources, [
    { from: "resources/${os}-${arch}/sshc.exe", to: "cli/sshc.exe" },
  ]);
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
  // oneClick は既定値に任せない。どちらであれ、選んだことを書き残す。
  assert.strictEqual(typeof nsis.oneClick, "boolean");
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
  assert.strictEqual(scripts["dist:win"], "electron-builder --win");
  assert.ok(!("dist" in scripts), "the combined dist script is still there");
});

// **Windows では外殻が端末側の入口を作らない。** 安定した場所も PATH も
// インストーラが持っているので、重ねて張ろうとしても失敗を報告するだけになる。
test("only the platforms without an installer manage their own CLI", () => {
  assert.strictEqual(managesItsOwnCLI("win32"), false);
  assert.strictEqual(managesItsOwnCLI("darwin"), true);
  assert.strictEqual(managesItsOwnCLI("linux"), true);
});

// 束に入るファイルの一覧から漏れると、実行時に require が失敗する。
test("the packaged files include every module main.js requires", () => {
  for (const module of [
    "installer.js",
    "launcher.js",
    "install-cli.js",
    "lifecycle.js",
  ]) {
    assert.ok(
      configuration.build.files.includes(module),
      `${module} is not packaged`,
    );
  }
});

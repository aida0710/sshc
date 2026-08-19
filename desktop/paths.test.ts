import assert from "node:assert/strict";
import { test } from "node:test";
import { existsSync } from "node:fs";
import { basename, dirname } from "node:path";
import { bundleRoot, repositoryBinary, resource } from "./paths.js";

// **この検査が守るのは、静かに壊れる方の失敗である。**
//
// TypeScript にしたことで、走る JS は out/ の中へ移った。図も package.json も
// installer.nsh も、その一つ上に置かれたままである。`join(__dirname, "build",
// …)` と書いていた場所は、**例外を出さずに間違った道を指す**ようになった——
// nativeImage.createFromPath は無い道に対して空の画像を返すだけなので、症状は
// 「図が Electron の既定に戻る」であり、テストもビルドも緑のまま出荷される。
//
// だから場所を知っているのは paths.ts ひとつにして、ここで実在を確かめる。

test("the bundle root is the directory that holds package.json", () => {
  assert.ok(
    existsSync(resource("package.json")),
    `package.json was not found at ${bundleRoot}`,
  );
  // out/ の中を指していないこと。**一つ上に居ることが、この module の主張である。**
  assert.notStrictEqual(basename(bundleRoot), "out");
});

// **束に入る資材は、束に入る前から在る。** ここが赤くなるのは、build/ の中身が
// 消えたか、resource() の起点がずれたときだけである。
test("every resource the shell reads at runtime is where paths says it is", () => {
  for (const asset of [
    ["build", "icon.png"],
    ["build", "trayTemplate.png"],
    ["build", "trayTemplate@2x.png"],
    ["build", "installer.nsh"],
  ]) {
    assert.ok(existsSync(resource(...asset)), `${asset.join("/")} is missing`);
  }
});

// **開発中に使う実体は、束の外にある。** checkout の bin/sshc であり、
// リポジトリの根から数えて外殻の一つ上である。
test("the development binary is looked for in the repository, not in the shell", () => {
  const binary = repositoryBinary();
  assert.strictEqual(basename(binary), "sshc");
  assert.strictEqual(basename(dirname(binary)), "bin");
  // desktop/ の隣ではなく、その親の下に居ること。
  assert.strictEqual(dirname(dirname(binary)), dirname(bundleRoot));
});

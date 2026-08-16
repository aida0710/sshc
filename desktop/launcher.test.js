"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const { mkdtemp, readFile, stat, access } = require("node:fs/promises");
const { join } = require("node:path");
const { tmpdir } = require("node:os");
const { recordLinuxLauncher } = require("./launcher");

async function descriptorIn() {
  const home = await mkdtemp(join(tmpdir(), "sshc-launcher-"));
  return { home, descriptor: join(home, ".ssh", "sshc", "desktop.json") };
}

const linux = { platform: "linux", packaged: true };

// **process.execPath ではなく APPIMAGE を書く。** 走っているのは一時マウントの
// 中身で、その道はアプリを閉じれば消える。
test("the descriptor records the AppImage the user actually has", async () => {
  const { descriptor } = await descriptorIn();

  const result = await recordLinuxLauncher({
    ...linux,
    appImage: "/home/a/Downloads/sshc-0.1.0.AppImage",
    descriptor,
  });

  assert.ok(result.recorded, result.reason);
  assert.deepEqual(JSON.parse(String(await readFile(descriptor))), {
    executable: "/home/a/Downloads/sshc-0.1.0.AppImage",
  });
});

test("the descriptor is private to the user", async () => {
  const { descriptor } = await descriptorIn();

  await recordLinuxLauncher({ ...linux, appImage: "/opt/sshc", descriptor });

  assert.strictEqual((await stat(descriptor)).mode & 0o777, 0o600);
});

// **相対パスは書かない。** 書けば、読んだ側にとって意味が作業ディレクトリで
// 変わる——起こすものが、どこで打ったかで変わってはならない。
test("nothing is recorded without an absolute AppImage path", async () => {
  for (const appImage of [undefined, "", "sshc.AppImage", "./sshc.AppImage"]) {
    const { descriptor } = await descriptorIn();

    const result = await recordLinuxLauncher({
      ...linux,
      appImage,
      descriptor,
    });

    assert.ok(!result.recorded, `${String(appImage)} was recorded`);
    await assert.rejects(
      () => access(descriptor),
      `${String(appImage)} left a descriptor behind`,
    );
  }
});

// 開発中に走っているのは checkout の Electron であって、利用者が後から起こせる
// ものではない。書けば、次に端末で打った人が消えた道を起こす。
test("a development run records nothing", async () => {
  const { descriptor } = await descriptorIn();

  const result = await recordLinuxLauncher({
    platform: "linux",
    packaged: false,
    appImage: "/home/a/checkout/node_modules/electron/dist/electron",
    descriptor,
  });

  assert.ok(!result.recorded);
  await assert.rejects(() => access(descriptor));
});

// macOS は束の id で起こすので、場所を覚える必要がない。
test("macOS records nothing, because it activates by bundle id", async () => {
  const { descriptor } = await descriptorIn();

  const result = await recordLinuxLauncher({
    platform: "darwin",
    packaged: true,
    appImage: "/Applications/sshc.app",
    descriptor,
  });

  assert.ok(!result.recorded);
  await assert.rejects(() => access(descriptor));
});

test("a later start overwrites the recorded location", async () => {
  const { descriptor } = await descriptorIn();
  await recordLinuxLauncher({ ...linux, appImage: "/old/sshc", descriptor });

  await recordLinuxLauncher({ ...linux, appImage: "/new/sshc", descriptor });

  assert.deepEqual(JSON.parse(String(await readFile(descriptor))), {
    executable: "/new/sshc",
  });
});

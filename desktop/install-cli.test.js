"use strict";

const assert = require("node:assert/strict");
const { test } = require("node:test");
const {
  mkdtemp,
  mkdir,
  writeFile,
  readFile,
  symlink,
  lstat,
  readlink,
  stat,
} = require("node:fs/promises");
const { join } = require("node:path");
const { tmpdir } = require("node:os");
const { installManagedCLI } = require("./install-cli");

/** workspace は、束・管理下・公開の三つの場所を持つ一時の家を作る。 */
async function workspace() {
  const home = await mkdtemp(join(tmpdir(), "sshc-install-"));
  const bundle = join(home, "mount", "resources");
  await mkdir(bundle, { recursive: true });
  const source = join(bundle, "sshc");
  await writeFile(source, "#!/bin/sh\necho v1\n", { mode: 0o755 });
  return {
    home,
    source,
    managed: join(home, ".local", "share", "sshc", "bin", "sshc"),
    public: join(home, ".local", "bin", "sshc"),
  };
}

const linux = { platform: "linux" };

// **公開の名前が束の中を指してはならない。** AppImage の中身は一時マウントで、
// アプリを閉じれば消える。次に `sshc` と打った人に残るのは壊れたリンクである。
test("the public name points at the managed copy, never into the bundle", async () => {
  const paths = await workspace();

  const result = await installManagedCLI({ ...paths, ...linux });

  assert.ok(result.copied, "the bundled CLI was not copied out");
  assert.ok(result.linked, "the public name was not created");
  assert.strictEqual(result.warning, null);
  assert.strictEqual(await readlink(paths.public), paths.managed);
  assert.strictEqual(
    String(await readFile(paths.managed)),
    "#!/bin/sh\necho v1\n",
  );
});

test("the managed copy and its directory are private and executable", async (t) => {
  // **Windows に写す mode ビットは無い。** 誰が読めるかを決めているのは DACL で
  // あり、Go 側の internal/platform/windowsacl がそちらを持つ。ここで同じ式を
  // 走らせると、落ちるだけでなく「ここにアクセス制御がある」という嘘が残る。
  // installManagedCLI は win32 では何もしないので、production の経路でもない。
  if (process.platform === "win32") {
    t.skip("Windows expresses this through the DACL, not through mode bits");
    return;
  }
  const paths = await workspace();

  await installManagedCLI({ ...paths, ...linux });

  const binary = await stat(paths.managed);
  assert.strictEqual(binary.mode & 0o777, 0o700);
  const directory = await stat(join(paths.home, ".local", "share", "sshc"));
  assert.strictEqual(directory.mode & 0o777, 0o700);
});

// 起動のたびに書き直すと、そのとき走っている sshc の inode が毎回入れ替わる。
test("an unchanged CLI is not rewritten on the next start", async () => {
  const paths = await workspace();
  await installManagedCLI({ ...paths, ...linux });
  const first = await stat(paths.managed);

  const second = await installManagedCLI({ ...paths, ...linux });

  assert.ok(!second.copied, "an identical CLI was copied again");
  assert.strictEqual((await stat(paths.managed)).ino, first.ino);
});

test("a changed CLI replaces the managed copy", async () => {
  const paths = await workspace();
  await installManagedCLI({ ...paths, ...linux });
  await writeFile(paths.source, "#!/bin/sh\necho v2 is longer\n", {
    mode: 0o755,
  });

  const result = await installManagedCLI({ ...paths, ...linux });

  assert.ok(result.copied);
  assert.strictEqual(
    String(await readFile(paths.managed)),
    "#!/bin/sh\necho v2 is longer\n",
  );
});

// **そこに何を置くかは利用者の決めたことである。** `make install` で入れた
// 実体を、断りなく symlink へ変えない。
test("an occupied public name is left alone and named in a warning", async () => {
  for (const [name, place] of [
    [
      "a real file",
      async (path) => {
        await writeFile(path, "someone else's sshc", { mode: 0o755 });
      },
    ],
    [
      "a link somewhere else",
      async (path) => {
        await symlink("/opt/other/sshc", path);
      },
    ],
    [
      "a broken link somewhere else",
      async (path) => {
        await symlink("/opt/gone/sshc", path);
      },
    ],
  ]) {
    const paths = await workspace();
    await mkdir(join(paths.home, ".local", "bin"), { recursive: true });
    await place(paths.public);
    const before = await lstat(paths.public);

    const result = await installManagedCLI({ ...paths, ...linux });

    assert.ok(!result.linked, `${name}: the public name was replaced`);
    assert.ok(result.warning !== null, `${name}: nothing was reported`);
    assert.ok(
      result.warning.includes(paths.public),
      `${name}: the warning does not name the path: ${result.warning}`,
    );
    assert.strictEqual(
      (await lstat(paths.public)).ino,
      before.ino,
      `${name}: what was there is gone`,
    );
    // 管理下の実体は、公開の名前が塞がっていても揃えておく。窓は動く。
    assert.ok(result.copied, `${name}: the managed copy was skipped too`);
  }
});

test("a public link that already points at the managed copy is left as it is", async () => {
  const paths = await workspace();
  await installManagedCLI({ ...paths, ...linux });

  const result = await installManagedCLI({ ...paths, ...linux });

  assert.ok(!result.linked, "the link was recreated");
  assert.strictEqual(result.warning, null);
  assert.strictEqual(await readlink(paths.public), paths.managed);
});

// **Windows で PATH を通すのはインストーラである。** ここで symlink を作ろう
// としても権限が要り、~/.local/bin は PATH に載っていない。
test("Windows installs nothing, because the installer owns the path there", async () => {
  const paths = await workspace();

  const result = await installManagedCLI({ ...paths, platform: "win32" });

  assert.deepEqual(result, {
    managed: null,
    copied: false,
    linked: false,
    warning: null,
    // **案内も出さない。** PATH を通すのはインストーラであって、ここではない。
    note: null,
  });
  await assert.rejects(() => lstat(paths.managed));
});

// 警告は利用者に見せるものである。**入口のトークンも保管庫の状態も入らない。**
test("the warning carries nothing but paths and an instruction", async () => {
  const paths = await workspace();
  await mkdir(join(paths.home, ".local", "bin"), { recursive: true });
  await writeFile(paths.public, "someone else's sshc");

  const { warning } = await installManagedCLI({ ...paths, ...linux });

  for (const secret of ["http://127.0.0.1", "token", "vault", "unlock"]) {
    assert.ok(
      !warning.toLowerCase().includes(secret),
      `the warning mentions ${secret}: ${warning}`,
    );
  }
});

// **リンクを張っただけでは `sshc` と打てるようにならない。**
//
// macOS の既定の PATH に `~/.local/bin` は無く、Debian 系の `~/.profile` が
// それを足すのはログインの時点でそのディレクトリが在ったときだけである——初回は
// この外殻がいま作ったのだから、在らなかった。`make install` は同じことを note と
// して言っており、こちらだけが黙っていた。
test("a newly created public name says where it went and to check PATH", async () => {
  const paths = await workspace();

  const result = await installManagedCLI({ ...paths, ...linux });

  assert.ok(result.linked, "the public name was not created");
  assert.ok(result.note !== null, "the installation said nothing about PATH");
  assert.ok(result.note.includes(paths.public), "the note did not name the command");
  assert.ok(result.note.includes("PATH"), "the note did not mention PATH");
});

// **既に整えた人に、同じ案内を繰り返さない。** 二度目の起動でリンクは既にあり、
// 張り直しは起きない。そこで案内を出せば、起動のたびにダイアログが出る。
test("an existing link repeats no advice", async () => {
  const paths = await workspace();
  await installManagedCLI({ ...paths, ...linux });

  const second = await installManagedCLI({ ...paths, ...linux });

  assert.ok(!second.linked, "the link was recreated");
  assert.strictEqual(second.note, null, "the advice was repeated on a later launch");
});


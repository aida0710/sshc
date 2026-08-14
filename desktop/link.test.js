"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { mkdtemp, writeFile, symlink, readlink, mkdir, rm } = require("node:fs/promises");
const { join } = require("node:path");
const { tmpdir } = require("node:os");

// relink は HOME を読むので、テストごとに使い捨ての HOME を与える。
// **本物の ~/.local/bin には決して触らない。**
async function withHome(run) {
  const home = await mkdtemp(join(tmpdir(), "sshc-link-"));
  const previous = process.env.HOME;
  process.env.HOME = home;
  // homedir() は起動時の値を覚えることがあるので、モジュールを読み直す。
  delete require.cache[require.resolve("./link")];
  const link = require("./link");
  try {
    await run(link, home);
  } finally {
    process.env.HOME = previous;
    await rm(home, { recursive: true, force: true });
  }
}

test("points the link at the bundled binary", async () => {
  await withHome(async (link, home) => {
    const target = join(home, "bundle", "sshc");
    await mkdir(join(home, "bundle"), { recursive: true });
    await writeFile(target, "#!/bin/sh\n");

    const result = await link.relink(target);
    assert.equal(result.changed, true, result.reason);
    assert.equal(await readlink(join(home, ".local", "bin", "sshc")), target);
  });
});

test("replaces a link that points somewhere else", async () => {
  await withHome(async (link, home) => {
    const path = join(home, ".local", "bin", "sshc");
    await mkdir(join(home, ".local", "bin"), { recursive: true });
    await symlink(join(home, "old"), path);

    const target = join(home, "new");
    await writeFile(target, "#!/bin/sh\n");
    assert.equal((await link.relink(target)).changed, true);
    assert.equal(await readlink(path), target);
  });
});

test("does nothing when it already points here", async () => {
  await withHome(async (link, home) => {
    const target = join(home, "sshc");
    await writeFile(target, "#!/bin/sh\n");
    await link.relink(target);
    const again = await link.relink(target);
    assert.equal(again.changed, false);
  });
});

// **`make install` で入れた実体を、断りなく symlink へ変えない。**
// その判断は人のものである。
test("leaves a real file alone", async () => {
  await withHome(async (link, home) => {
    const path = join(home, ".local", "bin", "sshc");
    await mkdir(join(home, ".local", "bin"), { recursive: true });
    await writeFile(path, "a real installed binary");

    const target = join(home, "bundle-sshc");
    await writeFile(target, "#!/bin/sh\n");
    const result = await link.relink(target);

    assert.equal(result.changed, false);
    assert.match(result.reason, /real file/);
    await assert.rejects(() => readlink(path), { code: "EINVAL" });
  });
});

// **リンクが張れないことは、アプリが開けない理由にはならない。**
test("reports a failure instead of throwing", async () => {
  await withHome(async (link, home) => {
    // ~/.local を普通のファイルにして、ディレクトリを作れなくする。
    await writeFile(join(home, ".local"), "not a directory");
    const result = await link.relink(join(home, "sshc"));
    assert.equal(result.changed, false);
    assert.notEqual(result.reason, "");
  });
});

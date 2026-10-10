import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import test from "node:test";
import { gunzipSync } from "node:zlib";
import { packageUI } from "../scripts/package-ui.mjs";
import { readTarFiles } from "../src/tar-archive.js";

const runFile = promisify(execFile);

test("配信用のUIアーカイブからHTMLと長いパスのバイナリを元の内容で取り出せる", async () => {
  const directory = await mkdtemp(join(tmpdir(), "sshc-ui-test-"));
  try {
    const sourceDirectory = join(directory, "ui");
    const nestedPath = `assets/${"a".repeat(90)}/font.woff2`;
    await mkdir(join(sourceDirectory, "assets", "a".repeat(90)), { recursive: true });
    await writeFile(join(sourceDirectory, "index.html"), "<head></head>");
    await writeFile(join(sourceDirectory, nestedPath), Uint8Array.of(0, 128, 255));
    const archive = await packageUI({ sourceDirectory, outputDirectory: directory });
    const tar = gunzipSync(await readFile(join(directory, archive.fileName)));
    const files = readTarFiles(tar);
    assert.equal(archive.fileCount, 2);
    assert.equal(files.length, 2);
    assert.deepEqual([...files.find((file) => file.path === nestedPath).bytes], [0, 128, 255]);
    assert.match(new TextDecoder().decode(files.find((file) => file.path === "index.html").bytes), /ui-bridge.js/);
    const damaged = Uint8Array.from(tar);
    damaged[0] ^= 1;
    assert.throws(() => readTarFiles(damaged), /checksum/);
    assert.throws(() => readTarFiles(tar.subarray(0, 700)), /Truncated|Missing tar end marker/);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("配信ディレクトリの外へ出るtarのファイル名を拒否する", async () => {
  const directory = await mkdtemp(join(tmpdir(), "sshc-ui-path-test-"));
  try {
    await writeFile(join(directory, "index.html"), "<head></head>");
    const { stdout } = await runFile("tar", ["--format=ustar", "--transform=s|index.html|../index.html|",
      "-cf", "-", "-C", directory, "index.html"], { encoding: "buffer" });
    assert.throws(() => readTarFiles(stdout), /file path/);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

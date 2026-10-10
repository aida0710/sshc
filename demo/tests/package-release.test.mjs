import assert from "node:assert/strict";
import test from "node:test";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { gunzipSync } from "node:zlib";
import { createHash } from "node:crypto";
import { packageRelease } from "../scripts/package-release.mjs";
import { readTarFiles } from "../src/tar-archive.js";

test("Releaseの束は同じ版の設定とバイナリを含みmanifestで検査できる", async () => {
  const folder = await mkdtemp(resolve(tmpdir(), "sshc-demo-package-"));
  try {
    const directory = resolve(folder, "demo");
    const outputDirectory = resolve(folder, "release");
    await mkdir(resolve(directory, "images"), { recursive: true });
    await writeFile(resolve(directory, "config.json"), JSON.stringify({ version: "v0.44.0", imageBaseURL: "./images/" }));
    await writeFile(resolve(directory, "index.html"), "<h1>demo</h1>");
    await writeFile(resolve(directory, "images/client.cpio.gz"), Uint8Array.of(0, 128, 255));
    const manifest = await packageRelease({ directory, outputDirectory });
    const compressed = await readFile(resolve(outputDirectory, manifest.fileName));
    assert.equal(manifest.version, "v0.44.0");
    assert.equal(manifest.bytes, compressed.length);
    assert.equal(manifest.sha256, createHash("sha256").update(compressed).digest("hex"));
    const files = readTarFiles(gunzipSync(compressed));
    assert.equal(files.length, manifest.fileCount);
    assert.equal(JSON.parse(new TextDecoder().decode(files.find(file => file.path === "config.json").bytes)).version, manifest.version);
    assert.deepEqual(Uint8Array.from(files.find(file => file.path === "images/client.cpio.gz").bytes), Uint8Array.of(0, 128, 255));
  } finally {
    await rm(folder, { recursive: true, force: true });
  }
});

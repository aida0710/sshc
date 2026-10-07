import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createTarGzip } from "./create-tar-gzip.mjs";

export async function packageRelease({ directory, outputDirectory }) {
  const configuration = JSON.parse(await readFile(resolve(directory, "config.json"), "utf8"));
  const version = configuration.version;
  if (!/^(?:dev|v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?)$/.test(version)) {
    throw new Error("Invalid demo version");
  }
  if (configuration.imageBaseURL !== "./images/") throw new Error("A release must include its own VM images");
  const archive = await createTarGzip(directory);
  const fileName = `sshc-demo-${version}.tar.gz`;
  const entries = await readdir(directory, { recursive: true, withFileTypes: true });
  const manifest = { format: 1, version, fileName, bytes: archive.length,
    fileCount: entries.filter((entry) => entry.isFile()).length,
    sha256: createHash("sha256").update(archive).digest("hex") };
  await mkdir(outputDirectory, { recursive: true });
  await writeFile(resolve(outputDirectory, fileName), archive);
  await writeFile(resolve(outputDirectory, `sshc-demo-${version}.json`), JSON.stringify(manifest) + "\n");
  return manifest;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const demoDirectory = fileURLToPath(new URL("..", import.meta.url));
  const manifest = await packageRelease({ directory: resolve(demoDirectory, "dist"),
    outputDirectory: resolve(process.argv[2] ?? resolve(demoDirectory, "artifacts/release")) });
  console.log(`Packaged ${manifest.fileName}: ${manifest.bytes} bytes, ${manifest.fileCount} files`);
}

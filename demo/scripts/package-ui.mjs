import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { cp, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { promisify } from "node:util";
import { gzipSync } from "node:zlib";

const runFile = promisify(execFile);
// The current UI is 8 MiB; allow room for future bundles without changing the tar format.
const maxArchiveBytes = 64 * 1024 * 1024;

export async function packageUI({ sourceDirectory, outputDirectory }) {
  const stagingDirectory = await mkdtemp(join(tmpdir(), "sshc-demo-ui-"));
  try {
    await cp(sourceDirectory, stagingDirectory, { recursive: true });
    const indexPath = join(stagingDirectory, "index.html");
    const nativeIndex = await readFile(indexPath, "utf8");
    // Only the demo copy installs the VM transport; the product build stays unchanged.
    await writeFile(indexPath, nativeIndex.replace("<head>", '<head><script src="../ui-bridge.js"></script>'));
    const { stdout } = await runFile("tar", ["--format=ustar", "--sort=name", "--mtime=@0",
      "--owner=0", "--group=0", "--numeric-owner", "-cf", "-", "-C", stagingDirectory, "."],
    { encoding: "buffer", maxBuffer: maxArchiveBytes });
    const archive = gzipSync(stdout, { level: 9 });
    const fileName = "ui.tar.gz";
    await writeFile(join(outputDirectory, fileName), archive);
    const entries = await readdir(stagingDirectory, { recursive: true, withFileTypes: true });
    return { fileName, bytes: archive.length, fileCount: entries.filter((entry) => entry.isFile()).length,
      sha256: createHash("sha256").update(archive).digest("hex") };
  } finally {
    await rm(stagingDirectory, { recursive: true, force: true });
  }
}

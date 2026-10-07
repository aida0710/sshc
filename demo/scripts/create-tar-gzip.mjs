import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { gzipSync } from "node:zlib";

const runFile = promisify(execFile);
// VM images are already compressed; this also allows room for future UI bundles.
const maxArchiveBytes = 128 * 1024 * 1024;

export async function createTarGzip(directory) {
  const { stdout } = await runFile("tar", ["--format=ustar", "--sort=name", "--mtime=@0",
    "--owner=0", "--group=0", "--numeric-owner", "-cf", "-", "-C", directory, "."],
  { encoding: "buffer", maxBuffer: maxArchiveBytes });
  return gzipSync(stdout, { level: 9 });
}

import { spawn } from "node:child_process";
import { mkdir, stat } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const { values } = parseArgs({ options: {
  release: { type: "string" }, manifest: { type: "string" }, rebuild: { type: "boolean", default: false },
} });
const demoDirectory = fileURLToPath(new URL("..", import.meta.url));
const timestamp = new Intl.DateTimeFormat("sv-SE", {
  timeZone: "Asia/Tokyo", year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23",
}).format(new Date()).replace(/[-: ]/g, "");
const release = values.release ?? timestamp;
const manifest = resolve(values.manifest ?? resolve(demoDirectory, "artifacts", `r2-manifest-${release}.json`));
if (!/^[A-Za-z0-9._-]+$/.test(release)) throw new Error("Invalid release name");
if (!process.env.SSHC_R2_CREDENTIALS_FILE) throw new Error("Set SSHC_R2_CREDENTIALS_FILE before deployment");
await stat(process.env.SSHC_R2_CREDENTIALS_FILE);
if (values.rebuild) {
  await runCommand("bash", [fileURLToPath(new URL("./build-images.sh", import.meta.url))]);
  await runCommand("npm", ["run", "build", "--prefix", resolve(demoDirectory, "../web"),
    "--", "--base=./", "--outDir", resolve(demoDirectory, "images/ui")]);
}
await import("./prepare.mjs");
await mkdir(dirname(manifest), { recursive: true });
await runCommand("python3", [fileURLToPath(new URL("./upload-r2.py", import.meta.url)),
  "--release", release, "--manifest", manifest]);
console.log(`Manifest: ${manifest}`);

function runCommand(command, commandArguments) {
  return new Promise((resolve, reject) => {
    const childProcess = spawn(command, commandArguments, { cwd: demoDirectory, stdio: "inherit" });
    childProcess.on("error", reject);
    childProcess.on("exit", (code) => code === 0 ? resolve() : reject(new Error(`${command} exited with ${code}`)));
  });
}

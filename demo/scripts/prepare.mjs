import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const demoDirectory = fileURLToPath(new URL("..", import.meta.url));
const outputDirectory = resolve(demoDirectory, "dist");
// dist is generated exclusively by this script; clearing it prevents stale UI bundles from being published.
await rm(outputDirectory, { recursive: true, force: true });
await mkdir(outputDirectory, { recursive: true });
await writeFile(resolve(outputDirectory, "config.json"), JSON.stringify({
  imageBaseURL: process.env.SSHC_DEMO_IMAGES_URL ?? "./images/",
}) + "\n");
await cp(resolve(demoDirectory, "src"), outputDirectory, { recursive: true });
await cp(resolve(demoDirectory, "images"), resolve(outputDirectory, "images"), {
  recursive: true,
  filter: (path) => !/\/(?:rootfs|ui)(?:\/|$)|\/(?:sshc|sshc-demo-bridge)$/.test(path),
});
await mkdir(resolve(outputDirectory, "v86"));
for (const filename of ["libv86.js", "v86.wasm", "v86-fallback.wasm"]) {
  await cp(resolve(demoDirectory, "node_modules/v86/build", filename), resolve(outputDirectory, "v86", filename));
}
await cp(resolve(demoDirectory, "node_modules/@xterm/xterm/lib/xterm.js"), resolve(outputDirectory, "xterm.js"));
await cp(resolve(demoDirectory, "node_modules/@xterm/xterm/css/xterm.css"), resolve(outputDirectory, "xterm.css"));
await cp(resolve(demoDirectory, "node_modules/@xterm/addon-fit/lib/addon-fit.js"), resolve(outputDirectory, "addon-fit.js"));
await mkdir(resolve(outputDirectory, "licenses"));
for (const [source, filename] of [
  ["../LICENSE", "sshc-Apache-2.0.txt"],
  ["node_modules/v86/LICENSE", "v86-BSD-2-Clause.txt"],
  ["node_modules/@xterm/xterm/LICENSE", "xterm-MIT.txt"],
  ["node_modules/@xterm/addon-fit/LICENSE", "addon-fit-MIT.txt"],
]) {
  await cp(resolve(demoDirectory, source), resolve(outputDirectory, "licenses", filename));
}
await cp(resolve(demoDirectory, "images/ui"), resolve(outputDirectory, "ui"), { recursive: true });
const nativeIndex = await readFile(resolve(outputDirectory, "ui/index.html"), "utf8");
// The production UI is reused verbatim; only this demo copy installs the VM transport.
await writeFile(resolve(outputDirectory, "ui/index.html"), nativeIndex
  .replace('<head>', '<head><script src="../ui-bridge.js"></script>'));
console.log(`Prepared ${outputDirectory}`);

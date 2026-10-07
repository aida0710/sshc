import { cp, mkdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { packageUI } from "./package-ui.mjs";

const demoDirectory = fileURLToPath(new URL("..", import.meta.url));
const outputDirectory = resolve(demoDirectory, "dist");
// dist is generated exclusively by this script; clearing it prevents stale UI bundles from being published.
await rm(outputDirectory, { recursive: true, force: true });
await mkdir(outputDirectory, { recursive: true });
const assetSizes = {};
for (const filename of ["seabios.bin", "vgabios.bin", "kernel.bin", "client.cpio.gz", "server.cpio.gz"]) {
  assetSizes[filename] = (await stat(resolve(demoDirectory, "images", filename))).size;
}
assetSizes["v86.wasm"] = (await stat(resolve(demoDirectory, "node_modules/v86/build/v86.wasm"))).size;
const uiArchive = await packageUI({ sourceDirectory: resolve(demoDirectory, "images/ui"), outputDirectory });
const { version } = JSON.parse(await readFile(resolve(demoDirectory, "images/build-version.json"), "utf8"));
await writeFile(resolve(outputDirectory, "config.json"), JSON.stringify({
  version,
  entryURL: "/index.html",
  releaseProxyURL: process.env.SSHC_DEMO_RELEASE_PROXY_URL ?? "https://sshc-demo-releases.aida0710.workers.dev/",
  imageBaseURL: process.env.SSHC_DEMO_IMAGES_URL ?? "./images/",
  assetSizes,
  uiArchive,
}) + "\n");
await cp(resolve(demoDirectory, "src"), outputDirectory, { recursive: true });
const indexPath = resolve(outputDirectory, "index.html");
const index = await readFile(indexPath, "utf8");
await writeFile(indexPath, index.replace("{{updatedAt}}", new Date().toISOString()).replace("{{version}}", version));
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
console.log(`Prepared ${outputDirectory}`);

import { connectVirtualLAN } from "./vm-network.js";
import { VMBridge } from "./vm-bridge.js";
import { machineDefinitions, loadDemoConfiguration, bundleDownloadBytes } from "./config.js";
import { applyMessages, messages } from "./messages.js";
import { StartupProgress } from "./startup-progress.js";
import { StartupProgressView } from "./startup-progress-view.js";
import { prepareUIArchive } from "./ui-archive.js";
import { UICacheControls } from "./ui-cache-controls.js";
import { consumeReleaseConsent, fetchLatestRelease, openRelease, deleteReleaseCache, shouldOpenRelease } from "./release-loader.js";

applyMessages(document);
const updatedAt = document.getElementById("updated-at");
updatedAt.textContent = messages.updatedAt(updatedAt.dateTime);
const uiBaseURL = new URL("./ui/", window.location.href);
const uiCacheControls = new UICacheControls({ button: document.getElementById("clear-ui-cache"),
  error: document.getElementById("cache-error"), baseURL: uiBaseURL,
  reload: async () => {
    const configuration = await loadDemoConfiguration();
    await deleteReleaseCache();
    window.location.assign(configuration.entryURL);
  },
});
uiCacheControls.refresh();
const initialConfiguration = await loadDemoConfiguration();
document.getElementById("demo-version").textContent = initialConfiguration.version;
const hasReleaseConsent = consumeReleaseConsent(initialConfiguration.version);
const releaseStatus = document.getElementById("release-status");
releaseStatus.textContent = initialConfiguration.releaseProxyURL && !hasReleaseConsent ? messages.checkingRelease : "";
// Until the latest lookup answers, the bundle served with this page is what would be downloaded.
const downloadTotal = document.getElementById("download-total");
downloadTotal.textContent = messages.downloadTotal(bundleDownloadBytes(initialConfiguration));
const latestReleasePromise = hasReleaseConsent ? Promise.resolve(null)
  : fetchLatestRelease(initialConfiguration.releaseProxyURL).then((manifest) => {
    if (manifest) releaseStatus.textContent = messages.latestRelease(manifest.version);
    if (shouldOpenRelease(manifest, initialConfiguration.version)) {
      downloadTotal.textContent = messages.downloadTotal(manifest.bytes);
    }
    return manifest;
  }).catch(() => {
    releaseStatus.textContent = messages.releaseFallback(initialConfiguration.version);
    return null;
  });
for (const definition of machineDefinitions) {
  const row = document.createElement("tr");
  for (const label of [definition.label, messages[definition.purpose], `${definition.memoryMiB} MiB`]) {
    const cell = document.createElement("td");
    cell.textContent = label;
    row.append(cell);
  }
  document.getElementById("machine-list").append(row);
}
document.getElementById("memory-detail").textContent = messages.memoryDetail(
  machineDefinitions.reduce((sum, definition) => sum + definition.memoryMiB, 0));
const status = document.getElementById("status");
const frame = document.getElementById("sshc-ui");
const startupProgress = new StartupProgress(machineDefinitions);
const startupView = new StartupProgressView(document.getElementById("startup"), machineDefinitions);
let bridge;
// The terminal takes its colors from demo.css so the frame and the terminal read as one surface.
const pageStyle = getComputedStyle(document.documentElement);
const terminal = new window.Terminal({
  cols: 100, rows: 28, fontSize: 14, cursorBlink: true,
  fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "DejaVu Sans Mono", monospace',
  theme: { background: pageStyle.getPropertyValue("--term-bg").trim(), foreground: pageStyle.getPropertyValue("--term-fg").trim() },
});
const fit = new window.FitAddon.FitAddon();
terminal.loadAddon(fit);
terminal.open(document.getElementById("cli"));
new ResizeObserver(() => {
  if (!document.getElementById("cli-panel").hidden) fit.fit();
}).observe(document.getElementById("cli"));
terminal.onResize(({ cols, rows }) => bridge?.send({ kind: "cli-resize", cols, rows }));

function selectTab(tab) {
  for (const name of ["web", "cli"]) {
    document.getElementById(`${name}-panel`).hidden = name !== tab;
    document.getElementById(`${name}-tab`).setAttribute("aria-pressed", String(name === tab));
  }
  if (tab === "cli") {
    fit.fit();
    terminal.focus();
  }
}
document.getElementById("web-tab").addEventListener("click", () => selectTab("web"));
document.getElementById("cli-tab").addEventListener("click", () => selectTab("cli"));
document.getElementById("reset").addEventListener("click", () => window.location.reload());

function renderStartupProgress() {
  const snapshot = startupProgress.snapshot;
  startupView.render(snapshot);
  if (snapshot.phase === "ready") {
    status.textContent = messages.ready;
    frame.hidden = false;
  } else if (snapshot.phase === "failed") {
    status.textContent = messages.failed;
    status.setAttribute("role", "alert");
  } else {
    status.textContent = snapshot.phase === "boot" ? messages.bootProgress(snapshot.bootedCount)
      : messages.startupPhases[snapshot.phase];
  }
}

// Only this demo's iframe can ask the VM bridge to access the guest loopback engine.
window.addEventListener("message", (event) => {
  if (event.origin !== window.location.origin || event.source !== frame.contentWindow) return;
  if (event.data?.channel !== "sshc-demo") return;
  if (event.data.startup) {
    if (event.data.startup === "ui-ready") startupProgress.markUIReady();
    else if (event.data.startup === "ui-failed") startupProgress.fail();
    renderStartupProgress();
    return;
  }
  bridge?.send(event.data.request);
});

async function startDemo() {
  document.getElementById("start").disabled = true;
  document.getElementById("confirmation").hidden = true;
  document.getElementById("demo").hidden = false;
  document.getElementById("demo-controls").hidden = false;
  document.getElementById("reset").hidden = false;
  startupView.start();
  renderStartupProgress();

  const latestRelease = await latestReleasePromise;
  if (shouldOpenRelease(latestRelease, initialConfiguration.version)) {
    startupProgress.startReleaseArchive({ version: latestRelease.version, bytes: latestRelease.bytes });
    renderStartupProgress();
    try {
      await openRelease({ manifest: latestRelease, onProgress: (progress) => {
        startupProgress.updateReleaseArchive(progress);
        renderStartupProgress();
      } });
      return;
    } catch {
      startupProgress.markReleaseArchiveUnavailable();
      renderStartupProgress();
    }
  }

  let imageBaseURL;
  try {
    const configuration = await loadDemoConfiguration();
    imageBaseURL = configuration.imageBaseURL;
    startupProgress.configureDownloads(configuration.assetSizes, configuration.uiArchive);
    renderStartupProgress();
    await prepareUIArchive({ archive: configuration.uiArchive, baseURL: uiBaseURL,
      onProgress: (progress) => {
        startupProgress.updateUIArchive(progress);
        renderStartupProgress();
      },
    });
    await uiCacheControls.refresh();
  } catch {
    startupProgress.fail();
    renderStartupProgress();
    return;
  }

  const machines = machineDefinitions.map((definition) => {
    const entropy = Array.from(crypto.getRandomValues(new Uint8Array(32)),
      (byte) => byte.toString(16).padStart(2, "0")).join("");
    return new window.V86({
      wasm_path: "./v86/v86.wasm",
      memory_size: definition.memoryMiB * 1024 * 1024,
      // The demo uses serial terminals, so reserve only the minimum VGA framebuffer.
      vga_memory_size: 512 * 1024,
      bios: { url: new URL("seabios.bin", imageBaseURL).href },
      vga_bios: { url: new URL("vgabios.bin", imageBaseURL).href },
      bzimage: { url: new URL("kernel.bin", imageBaseURL).href },
      initrd: { url: new URL(definition.image, imageBaseURL).href },
      cmdline: `console=ttyS0,115200 quiet rdinit=/init demo_role=${definition.role} demo_entropy=${entropy}`,
      uart1: definition.role === "client",
      autostart: false,
      net_device: { type: "ne2k" },
    });
  });
  // Expose emulator objects only for local browser verification, never their bridge messages.
  window.sshcDemo = { machines };
  connectVirtualLAN(machines);
  machines.forEach((machine, index) => {
    let bootLine = "";
    machine.add_listener("serial0-output-byte", (byte) => {
      if (index === 0) terminal.write(Uint8Array.of(byte));
      if (byte === 10) {
        if (bootLine.trim() === `SSHC_DEMO_BOOTED:${machineDefinitions[index].role}`) {
          startupProgress.markMachineBooted(machineDefinitions[index].role);
          renderStartupProgress();
        }
        bootLine = "";
      } else {
        bootLine += String.fromCharCode(byte);
      }
    });
    const role = machineDefinitions[index].role;
    machine.add_listener("download-progress", (event) => {
      startupProgress.updateDownload(role, event);
      renderStartupProgress();
    });
    machine.add_listener("emulator-ready", () => {
      if (startupProgress.snapshot.phase === "failed") return;
      startupProgress.markMachineStarted(role);
      renderStartupProgress();
      machine.run();
    });
    machine.add_listener("download-error", () => {
      startupProgress.fail(role);
      renderStartupProgress();
      for (const pendingMachine of machines) pendingMachine.stop();
    });
  });
  terminal.onData((input) => {
    for (const byte of new TextEncoder().encode(input)) machines[0].bus.send("serial0-input", byte);
  });
  bridge = new VMBridge(machines[0]);
  bridge.subscribe((reply) => {
    if (reply.kind === "ready") {
      startupProgress.markEngineReady();
      frame.src = `./ui/index.html#${reply.bootstrap}`;
      renderStartupProgress();
      return;
    }
    frame.contentWindow?.postMessage({ channel: "sshc-demo", reply }, window.location.origin);
  });
}

document.getElementById("start").addEventListener("click", startDemo);
if (hasReleaseConsent) startDemo();

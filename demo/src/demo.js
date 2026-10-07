import { connectVirtualLAN } from "./vm-network.js";
import { VMBridge } from "./vm-bridge.js";
import { machineDefinitions, loadImageBaseURL } from "./config.js";
import { applyMessages, messages } from "./messages.js";

applyMessages(document);
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
let bridge;
let bootedCount = 0;
const terminal = new window.Terminal({
  cols: 100, rows: 28, fontSize: 14, cursorBlink: true,
  theme: { background: "#0c1015", foreground: "#e5e9f0" },
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

// Only this demo's iframe can ask the VM bridge to access the guest loopback engine.
window.addEventListener("message", (event) => {
  if (event.origin !== window.location.origin || event.source !== frame.contentWindow) return;
  if (event.data?.channel !== "sshc-demo") return;
  bridge?.send(event.data.request);
});

document.getElementById("start").addEventListener("click", async () => {
  document.getElementById("start").disabled = true;
  document.getElementById("confirmation").hidden = true;
  document.getElementById("demo").hidden = false;
  document.getElementById("reset").hidden = false;
  status.textContent = messages.loading;

  let imageBaseURL;
  try {
    imageBaseURL = await loadImageBaseURL();
  } catch {
    status.textContent = messages.failed;
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
        if (bootLine.includes("SSHC_DEMO_BOOTED:")) {
          bootedCount++;
          status.textContent = messages.bootProgress(bootedCount);
        }
        bootLine = "";
      } else {
        bootLine += String.fromCharCode(byte);
      }
    });
    machine.add_listener("emulator-ready", () => machine.run());
    machine.add_listener("download-error", () => { status.textContent = messages.failed; });
  });
  terminal.onData((input) => {
    for (const byte of new TextEncoder().encode(input)) machines[0].bus.send("serial0-input", byte);
  });
  bridge = new VMBridge(machines[0]);
  bridge.subscribe((reply) => {
    if (reply.kind === "ready") {
      status.textContent = messages.ready;
      frame.src = `./ui/index.html#${reply.bootstrap}`;
      frame.hidden = false;
      return;
    }
    frame.contentWindow?.postMessage({ channel: "sshc-demo", reply }, window.location.origin);
  });
});

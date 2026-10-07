const sharedBootFiles = ["v86.wasm", "seabios.bin", "vgabios.bin", "kernel.bin"];

// Download completion, Linux boot, engine readiness and UI readiness are separate signals.
export class StartupProgress {
  #machines;
  #isConfigured = false;
  #isEngineReady = false;
  #isUIReady = false;
  #hasFailed = false;
  #uiArchive = { size: 0, fraction: 0, state: "waiting" };

  constructor(definitions) {
    this.#machines = new Map(definitions.map((definition) => [definition.role, {
      role: definition.role, label: definition.label, state: "waiting", currentFile: null,
      files: new Map([...sharedBootFiles, definition.image].map((name) => [name, { size: 0, fraction: 0 }])),
    }]));
  }

  configureDownloads(assetSizes, archive) {
    this.#isConfigured = true;
    this.#uiArchive = archive ? { size: archive.bytes, fraction: 0, state: "downloading" }
      : { size: 0, fraction: 1, state: "ready" };
    for (const machine of this.#machines.values()) {
      machine.state = "downloading";
      for (const [name, file] of machine.files) file.size = assetSizes[name] ?? 0;
    }
  }

  updateUIArchive(progress) {
    if (this.#hasFailed) return;
    Object.assign(this.#uiArchive, progress);
  }

  updateDownload(role, event) {
    const machine = this.#machines.get(role);
    if (this.#hasFailed || machine.state !== "downloading") return;
    const name = event.file_name.split("/").at(-1).split("?")[0];
    const file = machine.files.get(name);
    if (!file) return;
    // v86 reads files sequentially. Its WASM and BIOS both use file_index=0,
    // so names identify files and a new name confirms the preceding read completed.
    if (machine.currentFile && machine.currentFile !== name) {
      machine.files.get(machine.currentFile).fraction = 1;
    }
    machine.currentFile = name;
    if (!file.size && event.total > 0) file.size = event.total;
    const total = event.total > 0 ? event.total : file.size;
    if (total > 0) file.fraction = Math.max(file.fraction, Math.min(1, event.loaded / total));
  }

  markMachineStarted(role) {
    const machine = this.#machines.get(role);
    if (this.#hasFailed) return;
    machine.state = "booting";
    for (const file of machine.files.values()) file.fraction = 1;
  }

  markMachineBooted(role) {
    if (this.#hasFailed) return;
    this.#machines.get(role).state = "booted";
  }

  markEngineReady() { this.#isEngineReady = true; }
  markUIReady() { this.#isUIReady = true; }

  fail(role) {
    this.#hasFailed = true;
    if (role) this.#machines.get(role).state = "failed";
  }

  get snapshot() {
    const machines = [...this.#machines.values()];
    const bootedCount = machines.filter((machine) => machine.state === "booted").length;
    let phase = "ready";
    if (this.#hasFailed) phase = "failed";
    else if (!this.#isConfigured) phase = "configuration";
    else if (this.#uiArchive.state !== "ready" || machines.some((machine) => machine.state === "downloading")) phase = "download";
    else if (bootedCount < machines.length) phase = "boot";
    else if (!this.#isEngineReady) phase = "engine";
    else if (!this.#isUIReady) phase = "web";
    const allFiles = [this.#uiArchive, ...machines.flatMap((machine) => [...machine.files.values()])];
    return {
      phase, bootedCount, downloadPercentage: downloadPercentage(allFiles),
      uiArchive: { ...this.#uiArchive, downloadPercentage: Math.floor(this.#uiArchive.fraction * 100) },
      machines: machines.map((machine) => ({
        role: machine.role, label: machine.label,
        state: machine.state === "booted"
          ? (machine.role === "client" && !this.#isEngineReady ? "engine" : "ready")
          : machine.state,
        downloadPercentage: downloadPercentage([...machine.files.values()]),
      })),
    };
  }
}

function downloadPercentage(files) {
  const total = files.reduce((sum, file) => sum + file.size, 0);
  if (total === 0) return null;
  const loaded = files.reduce((sum, file) => sum + file.size * file.fraction, 0);
  return Math.floor(loaded * 100 / total);
}

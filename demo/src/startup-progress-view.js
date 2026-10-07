import { messages } from "./messages.js";

// Elapsed time is informative while the emulator boots; it is not a completion estimate.
const elapsedRefreshMilliseconds = 1000;
const startupPhases = ["download", "boot", "engine", "web"];

export class StartupProgressView {
  #root;
  #machineRows = new Map();
  #startedAt;
  #elapsedTimer;

  constructor(root, definitions) {
    this.#root = root;
    for (const label of messages.startupStages) {
      const step = document.createElement("li");
      step.textContent = label;
      root.querySelector("ol").append(step);
    }
    for (const definition of definitions) {
      const row = document.createElement("li");
      row.dataset.machine = definition.role;
      const heading = document.createElement("div");
      heading.className = "startup-machine-heading";
      const name = document.createElement("strong");
      name.textContent = definition.label;
      const state = document.createElement("span");
      heading.append(name, state);
      const bar = document.createElement("progress");
      bar.max = 100;
      bar.setAttribute("aria-label", messages.machineDownload(definition.label));
      row.append(heading, bar);
      root.querySelector("ul").append(row);
      this.#machineRows.set(definition.role, { row, state, bar });
    }
  }

  start() {
    this.#root.hidden = false;
    this.#startedAt = performance.now();
    this.#updateElapsed();
    this.#elapsedTimer = setInterval(() => this.#updateElapsed(), elapsedRefreshMilliseconds);
  }

  render(snapshot) {
    const stepIndex = snapshot.phase === "ready" ? startupPhases.length
      : Math.max(0, startupPhases.indexOf(snapshot.phase));
    for (const [index, step] of [...this.#root.querySelectorAll("ol li")].entries()) {
      step.dataset.state = index < stepIndex ? "complete" : index === stepIndex ? "current" : "waiting";
      if (index === stepIndex) step.setAttribute("aria-current", "step");
      else step.removeAttribute("aria-current");
    }
    const bar = this.#root.querySelector("#startup-download");
    const percentage = snapshot.downloadPercentage;
    if (percentage === null) bar.removeAttribute("value");
    else bar.value = percentage;
    this.#root.querySelector("#startup-download-label").textContent = messages.downloadProgress(percentage);
    this.#root.querySelector("#startup-ui-label").textContent = messages.uiArchiveProgress(snapshot.uiArchive);
    this.#root.querySelector("#startup-ui-download").value = snapshot.uiArchive.downloadPercentage;
    for (const machine of snapshot.machines) {
      const { row, state, bar: machineBar } = this.#machineRows.get(machine.role);
      row.dataset.state = machine.state;
      state.textContent = machine.state === "downloading"
        ? messages.machineLoading(machine.downloadPercentage) : messages.machineStates[machine.state];
      if (machine.downloadPercentage === null) machineBar.removeAttribute("value");
      else machineBar.value = machine.downloadPercentage;
    }
    if (snapshot.phase === "ready" || snapshot.phase === "failed") {
      clearInterval(this.#elapsedTimer);
      this.#updateElapsed();
    }
    this.#root.hidden = snapshot.phase === "ready";
  }

  #updateElapsed() {
    const seconds = Math.floor((performance.now() - this.#startedAt) / 1000);
    this.#root.querySelector("#startup-elapsed").textContent = messages.elapsed(seconds);
  }
}

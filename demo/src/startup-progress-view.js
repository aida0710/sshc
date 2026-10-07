import { messages } from "./messages.js";

// Elapsed time is informative while the emulator boots; it is not a completion estimate.
const elapsedRefreshMilliseconds = 1000;
const startupPhases = ["download", "boot", "engine", "web"];

export class StartupProgressView {
  #root;
  #releaseRow;
  #uiRow;
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
    const archiveList = root.querySelector(".startup-archives");
    this.#releaseRow = createProgressRow(archiveList, "");
    this.#releaseRow.row.hidden = true;
    this.#uiRow = createProgressRow(archiveList, messages.uiArchiveLabel);
    this.#uiRow.bar.setAttribute("aria-label", messages.archiveDownload(messages.uiArchiveLabel));
    for (const definition of definitions) {
      const machineRow = createProgressRow(root.querySelector(".startup-machines"), definition.label);
      machineRow.row.dataset.machine = definition.role;
      machineRow.bar.setAttribute("aria-label", messages.machineDownload(definition.label));
      this.#machineRows.set(definition.role, machineRow);
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
    const percentage = snapshot.downloadPercentage;
    setProgress(this.#root.querySelector("#startup-download"), percentage);
    this.#root.querySelector("#startup-download-label").textContent = messages.downloadProgress(percentage);
    if (snapshot.releaseArchive) this.#renderReleaseArchive(snapshot.releaseArchive);
    renderArchiveRow(this.#uiRow, snapshot.uiArchive);
    for (const machine of snapshot.machines) {
      const { row, state, bar } = this.#machineRows.get(machine.role);
      row.dataset.state = machine.state;
      state.textContent = machine.state === "downloading"
        ? messages.machineLoading(machine.downloadPercentage) : messages.machineStates[machine.state];
      // A machine that has not started reading files is idle, not of unknown progress.
      setProgress(bar, machine.state === "waiting" ? 0 : machine.downloadPercentage);
    }
    if (snapshot.phase === "ready" || snapshot.phase === "failed") {
      clearInterval(this.#elapsedTimer);
      this.#updateElapsed();
    }
    this.#root.hidden = snapshot.phase === "ready";
  }

  #renderReleaseArchive(releaseArchive) {
    const label = messages.releaseArchiveLabel(releaseArchive.version);
    this.#releaseRow.row.hidden = false;
    this.#releaseRow.name.textContent = label;
    this.#releaseRow.bar.setAttribute("aria-label", messages.archiveDownload(label));
    renderArchiveRow(this.#releaseRow, releaseArchive);
  }

  #updateElapsed() {
    const seconds = Math.floor((performance.now() - this.#startedAt) / 1000);
    this.#root.querySelector("#startup-elapsed").textContent = messages.elapsed(seconds);
  }
}

function createProgressRow(list, label) {
  const row = document.createElement("li");
  const heading = document.createElement("div");
  heading.className = "startup-item-heading";
  const name = document.createElement("strong");
  name.textContent = label;
  const state = document.createElement("span");
  heading.append(name, state);
  const bar = document.createElement("progress");
  bar.max = 100;
  row.append(heading, bar);
  list.append(row);
  return { row, name, state, bar };
}

function renderArchiveRow({ row, state, bar }, archive) {
  row.dataset.state = archive.state;
  state.textContent = messages.archiveStates(archive);
  setProgress(bar, archive.state === "unavailable" ? 0 : archive.downloadPercentage);
}

function setProgress(bar, percentage) {
  if (percentage === null) bar.removeAttribute("value");
  else bar.value = percentage;
}

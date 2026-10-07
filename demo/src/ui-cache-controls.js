import { uiCacheName } from "./ui-cache-addresses.js";
import { messages } from "./messages.js";

export class UICacheControls {
  #button;
  #error;
  #baseURL;
  #reload;

  constructor({ button, error, baseURL, reload }) {
    this.#button = button;
    this.#error = error;
    this.#baseURL = baseURL;
    this.#reload = reload;
    button.addEventListener("click", () => this.#clearAndReload());
  }

  async refresh() {
    try {
      this.#button.hidden = !globalThis.caches || !await caches.has(uiCacheName(this.#baseURL));
    } catch {
      this.#button.hidden = true;
    }
  }

  async #clearAndReload() {
    this.#button.disabled = true;
    this.#error.hidden = true;
    try {
      await caches.delete(uiCacheName(this.#baseURL));
      await this.#reload();
    } catch {
      this.#error.textContent = messages.cacheClearFailed;
      this.#error.hidden = false;
      this.#button.disabled = false;
    }
  }
}

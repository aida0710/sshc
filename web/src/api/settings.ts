import { apiClient } from "./client";
import { asArray, asNumber, asRecord, asString, jsonHeaders } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type TerminalSettings = components["schemas"]["TerminalSettings"];
export type LocalShellProfile = components["schemas"]["LocalShellProfile"];
export type LocalShellProfileList = components["schemas"]["LocalShellProfileList"];
export type EngineSettings = components["schemas"]["EngineSettings"];
export type TerminalAppearance = components["schemas"]["TerminalAppearance"];
export type TerminalBackground = components["schemas"]["TerminalBackground"];
export type TerminalBackgroundList = components["schemas"]["TerminalBackgroundList"];

function readAppearance(value: unknown): TerminalAppearance {
  const record = asRecord(value);
  return {
    ...(typeof record.palette === "string" ? { palette: record.palette } : {}),
    ...(typeof record.font === "string" ? { font: record.font } : {}),
    ...(typeof record.background === "string"
      ? { background: record.background }
      : {}),
    ...(typeof record.backgroundTint === "number"
      ? { backgroundTint: record.backgroundTint }
      : {}),
  };
}

export type SettingsApi = {
  terminalSettings(): Promise<TerminalSettings>;
  setTerminalSettings(settings: TerminalSettings): Promise<void>;
  localShellProfiles?(): Promise<LocalShellProfileList>;
  engineSettings(): Promise<EngineSettings>;
  setEngineSettings(settings: EngineSettings): Promise<void>;
  terminalBackgrounds(): Promise<TerminalBackgroundList>;
  addTerminalBackground(
    suggested: string,
    image: Blob,
  ): Promise<TerminalBackground>;
  setTerminalBackgroundCapacity(capacityMiB: number): Promise<TerminalBackgroundList>;
  renameTerminalBackground(name: string, nextName: string): Promise<TerminalBackground>;
  deleteTerminalBackground(name: string): Promise<void>;
};

function validateLocalShellProfiles(value: unknown): LocalShellProfileList {
  return validateOpenAPISchema<LocalShellProfileList>("LocalShellProfileList", value);
}

function validateBackground(value: unknown): TerminalBackground {
  return validateOpenAPISchema<TerminalBackground>("TerminalBackground", value);
}

// Settings the engine stores for every browser: the embedded terminal, the
// engine itself, the local shell profiles it found, and the background images.
export const settingsApi: SettingsApi = {
  async terminalSettings() {
    const metadata = asRecord(await apiClient.read("/api/v1/metadata"));
    if (metadata.embeddedTerminal === undefined) return {};
    const terminal = asRecord(metadata.embeddedTerminal);
    return {
      ...(typeof terminal.startDirectory === "string" &&
      terminal.startDirectory !== ""
        ? { startDirectory: terminal.startDirectory }
        : {}),
      ...(typeof terminal.maxSessions === "number"
        ? { maxSessions: terminal.maxSessions }
        : {}),
      ...(typeof terminal.scrollbackBytes === "number"
        ? { scrollbackBytes: terminal.scrollbackBytes }
        : {}),
      ...(typeof terminal.browserScrollbackLines === "number" &&
      terminal.browserScrollbackLines >= 1000 &&
      terminal.browserScrollbackLines <= 100000
        ? { browserScrollbackLines: terminal.browserScrollbackLines }
        : {}),
      ...(typeof terminal.fontSize === "number"
        ? { fontSize: terminal.fontSize }
        : {}),
      ...(typeof terminal.verbosity === "number"
        ? { verbosity: terminal.verbosity }
        : {}),
      ...(typeof terminal.reconnect === "number"
        ? { reconnect: terminal.reconnect }
        : {}),
      ...(typeof terminal.copyOnSelect === "boolean"
        ? { copyOnSelect: terminal.copyOnSelect }
        : {}),
      ...(typeof terminal.rightClickPaste === "boolean"
        ? { rightClickPaste: terminal.rightClickPaste }
        : {}),
      ...(typeof terminal.webgl === "boolean" ? { webgl: terminal.webgl } : {}),
      ...(typeof terminal.osc52 === "boolean" ? { osc52: terminal.osc52 } : {}),
      ...(typeof terminal.jisYenBackslash === "boolean"
        ? { jisYenBackslash: terminal.jisYenBackslash }
        : {}),
      ...(typeof terminal.localShellProfile === "string" &&
      /^[a-z0-9-]{1,64}$/.test(terminal.localShellProfile)
        ? { localShellProfile: terminal.localShellProfile }
        : {}),
      ...(terminal.appearance === undefined
        ? {}
        : { appearance: readAppearance(terminal.appearance) }),
    };
  },
  async setTerminalSettings(settings) {
    await apiClient.mutate("/api/v1/metadata/terminal", {
      method: "PUT",
      headers: jsonHeaders,
      body: JSON.stringify(settings),
    });
  },
  async localShellProfiles() {
    return validateLocalShellProfiles(
      await apiClient.read("/api/v1/terminal/shell-profiles"),
    );
  },
  async engineSettings() {
    const metadata = asRecord(await apiClient.read("/api/v1/metadata"));
    if (metadata.engine === undefined) return {};
    const engine = asRecord(metadata.engine);
    const settings: EngineSettings = typeof engine.port === "number" ? { port: engine.port } : {};
    if (engine.vaultAutoLock !== undefined) {
      const autoLock = asRecord(engine.vaultAutoLock);
      const mode = asString(autoLock.mode);
      if (mode === "restart") {
        settings.vaultAutoLock = { mode };
      } else if (mode === "idle") {
        const value = asNumber(autoLock.value);
        const unit = asString(autoLock.unit);
        if (Number.isSafeInteger(value) && value >= 1 && value <= 999 &&
            (unit === "minutes" || unit === "hours")) {
          settings.vaultAutoLock = { mode, value, unit };
        }
      }
    }
    return settings;
  },
  async setEngineSettings(settings) {
    await apiClient.mutate("/api/v1/metadata/engine", {
      method: "PUT",
      headers: jsonHeaders,
      body: JSON.stringify(settings),
    });
  },
  async terminalBackgrounds() {
    const record = asRecord(
      await apiClient.read("/api/v1/terminal/backgrounds"),
    );
    return {
      backgrounds: asArray(record.backgrounds).map(validateBackground),
      usedBytes: asNumber(record.usedBytes),
      capacityBytes: asNumber(record.capacityBytes),
      remainingBytes: asNumber(record.remainingBytes),
    };
  },
  async addTerminalBackground(suggested, image) {
    return validateBackground(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/backgrounds?name=${encodeURIComponent(suggested)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/octet-stream" },
          body: image,
        },
      ),
    );
  },
  async setTerminalBackgroundCapacity(capacityMiB) {
    const record = asRecord(await apiClient.mutate<unknown>(
      "/api/v1/terminal/backgrounds/capacity",
      { method: "PUT", headers: jsonHeaders, body: JSON.stringify({ capacityMiB }) },
    ));
    return {
      backgrounds: asArray(record.backgrounds).map(validateBackground),
      usedBytes: asNumber(record.usedBytes),
      capacityBytes: asNumber(record.capacityBytes),
      remainingBytes: asNumber(record.remainingBytes),
    };
  },
  async renameTerminalBackground(name, nextName) {
    return validateBackground(
      await apiClient.mutate<unknown>(
        `/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`,
        {
          method: "PATCH",
          headers: jsonHeaders,
          body: JSON.stringify({ name: nextName }),
        },
      ),
    );
  },
  async deleteTerminalBackground(name) {
    await apiClient.mutate<unknown>(
      `/api/v1/terminal/backgrounds/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    );
  },
};

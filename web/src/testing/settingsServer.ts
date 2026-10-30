import { vi } from "vitest";
import { apiClient } from "../api/client";
import { resetSettingsForTests, standardPresetId, type SettingsDocument } from "../settings/settingsStore";

// The client's own read and write, kept before any test replaces them: a second fake server in the same test
// would otherwise pass on to the first one's replacement, which is now itself.
const clientRead = apiClient.read.bind(apiClient);
const clientMutate = apiClient.mutate.bind(apiClient);

// The machine the tests' engine runs on.
export const testMachineId = "0123456789abcdef0123456789abcdef";

// settingsDocument is what GET /api/v1/settings answers for a machine that has chosen nothing, with patch on top.
export function settingsDocument(patch: Partial<SettingsDocument> = {}): SettingsDocument {
  return {
    machineId: testMachineId,
    global: {},
    presets: [{ id: standardPresetId }],
    presetInUse: standardPresetId,
    machines: [{ id: testMachineId, settings: { name: "desk" }, facts: { os: "linux" } }],
    state: { facts: { os: "linux" } },
    ...patch,
  };
}

export type SettingsRequest = { method: string; path: string; body: unknown };

// fakeSettingsServer answers the settings endpoints the way the engine does, keeps what is written, and puts
// the document into the settings store, as the app does once the engine is ready. Other reads and writes go to
// fallback, or to the client as it was.
export function fakeSettingsServer(initial: Partial<SettingsDocument> = {}, fallback: {
  read?: (path: string) => Promise<unknown>;
  mutate?: (path: string, init: RequestInit) => Promise<unknown>;
} = {}) {
  let document = settingsDocument(initial);
  const requests: SettingsRequest[] = [];
  const resolvePresetInUse = () => {
    const chosen = document.machines.find((machine) => machine.id === document.machineId)?.settings.presetId;
    document.presetInUse = document.presets.some((preset) => preset.id === chosen) ? chosen ?? standardPresetId : standardPresetId;
  };
  vi.spyOn(apiClient, "read").mockImplementation(async (path: string) => {
    if (path === "/api/v1/settings") return structuredClone(document);
    return fallback.read === undefined ? clientRead(path) : fallback.read(path);
  });
  vi.spyOn(apiClient, "mutate").mockImplementation(async (path: string, init: RequestInit) => {
    if (!path.startsWith("/api/v1/settings")) return fallback.mutate === undefined ? clientMutate(path, init) : fallback.mutate(path, init);
    const body: unknown = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    requests.push({ method: init?.method ?? "GET", path, body });
    const sent = (body ?? {}) as {
      settings?: unknown;
      presets?: SettingsDocument["presets"];
      preset?: SettingsDocument["presets"][number];
      interface?: Record<string, string>;
      transfersStopped?: boolean;
    };
    if (path === "/api/v1/settings/global") document.global = sent.settings as SettingsDocument["global"];
    else if (path === "/api/v1/settings/presets") document.presets = sent.presets ?? document.presets;
    else if (path.startsWith("/api/v1/settings/presets/")) {
      const preset = sent.preset;
      if (preset !== undefined) document.presets = document.presets.map((candidate) => candidate.id === preset.id ? preset : candidate);
    } else if (path.startsWith("/api/v1/settings/machines/")) {
      const id = decodeURIComponent(path.slice("/api/v1/settings/machines/".length));
      const settings = sent.settings as SettingsDocument["machines"][number]["settings"];
      if (init?.method === "DELETE") document.machines = document.machines.filter((machine) => machine.id !== id);
      else document.machines = document.machines.map((machine) => machine.id === id ? { ...machine, settings } : machine);
    } else if (path === "/api/v1/settings/state") {
      const next: Record<string, string> = { ...document.state.interface };
      for (const [key, value] of Object.entries(sent.interface ?? {})) {
        if (value === "") delete next[key];
        else next[key] = value;
      }
      document.state = { ...document.state, interface: next, ...(sent.transfersStopped === undefined ? {} : { transfersStopped: sent.transfersStopped }) };
    }
    resolvePresetInUse();
    return { transactionId: "test", written: [] };
  });
  resetSettingsForTests(structuredClone(document));
  return {
    requests,
    // writesTo is the bodies written to path, oldest first.
    writesTo: (path: string) => requests.filter((request) => request.path === path).map((request) => request.body),
    document: () => document,
  };
}

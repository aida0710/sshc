import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { configApi } from "../api/config";
import type { TerminalSession } from "../api/terminalSessions";
import { useOSC52Policy } from "./useOSC52Policy";

vi.mock("../api/config", async () => {
  const actual = await vi.importActual<typeof import("../api/config")>("../api/config");
  return { ...actual, configApi: { overview: vi.fn(), save: vi.fn() } };
});

const bastion = { path: "config", alias: "bastion" };

describe("useOSC52Policy", () => {
  it("saves only the SSH connection's own metadata, with the entry it read as the base", async () => {
    vi.mocked(configApi.overview).mockResolvedValue({
      hosts: [{ identity: bastion }],
      metadata: {
        schemaVersion: 8,
        hosts: [{ identity: bastion, tags: ["prod"] }],
        vpnProfiles: [{ name: "office" }],
      },
    } as never);
    vi.mocked(configApi.save).mockResolvedValue({} as never);
    const setHostPolicy = vi.fn();
    const { result } = renderHook(() => useOSC52Policy({ settings: {}, setSettings: vi.fn(), setHostPolicy }));

    await result.current({ kind: "ssh", alias: "bastion" } as TerminalSession, true);

    expect(configApi.save).toHaveBeenCalledWith({
      kind: "metadata",
      path: "config",
      alias: "bastion",
      hostMetadataBase: { identity: bastion, tags: ["prod"] },
      hostMetadata: { identity: bastion, tags: ["prod"], osc52: "allow" },
    });
    expect(setHostPolicy).toHaveBeenCalledTimes(1);
  });
});

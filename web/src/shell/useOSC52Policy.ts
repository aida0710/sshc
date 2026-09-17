import type { Dispatch, SetStateAction } from "react";
import { configApi } from "../api/config";
import { settingsApi, type TerminalSettings } from "../api/settings";
import type { TerminalSession } from "../api/terminalSessions";

// Where a console's OSC 52 (clipboard) choice is kept: a local shell changes
// the terminal-wide default, an SSH session records a policy for its host in
// the connection metadata.
export function useOSC52Policy({ settings, setSettings, setHostPolicy }: {
  settings: TerminalSettings;
  setSettings: (settings: TerminalSettings) => void;
  setHostPolicy: Dispatch<SetStateAction<Map<string, "allow" | "deny">>>;
}) {
  return async function changeOSC52(session: TerminalSession, enabled: boolean): Promise<void> {
    if (session.kind !== "ssh" || session.alias === undefined) {
      const next = { ...settings };
      if (enabled) next.osc52 = true;
      else delete next.osc52;
      await settingsApi.setTerminalSettings(next);
      setSettings(next);
      return;
    }
    const alias = session.alias;
    const overview = await configApi.overview();
    const identity = overview.hosts.find((host) => host.identity.alias === alias)?.identity;
    if (identity === undefined) throw new Error("host_not_found");
    const hosts = [...(overview.metadata.hosts ?? [])];
    const index = hosts.findIndex(
      (host) => host.identity.path === identity.path && host.identity.alias === identity.alias,
    );
    const updated = {
      ...(index < 0 ? {} : hosts[index]),
      identity,
      osc52: enabled ? ("allow" as const) : ("deny" as const),
    };
    if (index < 0) hosts.push(updated);
    else hosts[index] = updated;
    await configApi.save({ kind: "metadata", metadata: { ...overview.metadata, hosts } });
    setHostPolicy((current) => new Map(current).set(alias, updated.osc52));
  };
}

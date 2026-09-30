import { useCallback, type Dispatch, type SetStateAction } from "react";
import { configApi, hostMetadataEditRequest } from "../api/config";
import { settingsApi, type TerminalSettings } from "../api/settings";
import type { TerminalSession } from "../api/terminalSessions";
import { hostMetadataByIdentity, identityKey } from "../connections/connectionBrowser";

// Where a session's OSC 52 (clipboard) choice is kept: a local shell changes
// the terminal-wide default, an SSH session records a policy for its host in
// the connection metadata.
export function useOSC52Policy({ settings, setSettings, setHostPolicy }: {
  settings: TerminalSettings;
  setSettings: (settings: TerminalSettings) => void;
  setHostPolicy: Dispatch<SetStateAction<Map<string, "allow" | "deny">>>;
}) {
  return useCallback(async (session: TerminalSession, enabled: boolean): Promise<void> => {
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
    const base = hostMetadataByIdentity(overview.metadata.hosts).get(identityKey(identity)) ?? { identity };
    const updated = { ...base, identity, osc52: enabled ? ("allow" as const) : ("deny" as const) };
    await configApi.save(hostMetadataEditRequest(base, updated));
    setHostPolicy((current) => new Map(current).set(alias, updated.osc52));
  }, [settings, setSettings, setHostPolicy]);
}

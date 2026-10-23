import { useCallback, useEffect, useState } from "react";
import { remoteEnginesApi, type RemoteEnginesApi } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { failureCode } from "../api/client";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { runTargetProblemText } from "./runTargetProblem";
import { useRemoteVaultUnlock, type RemoteVaultUnlockState } from "./useRemoteVaultUnlock";

export type RunTargetsState = {
  // engines are the SSH hosts whose engine was added as a run target.
  engines: string[];
  // selected is the run target whose terminals the sidebar and terminal show;
  // null is this machine.
  selected: string | null;
  problem: string;
  busy: boolean;
  select: (alias: string | null) => void;
  add: (alias: string) => Promise<boolean>;
  remove: (alias: string) => Promise<void>;
  openInNewTab: (alias: string) => Promise<void>;
  // api and localVersion are what the engine information dialog asks with.
  api: RemoteEnginesApi;
  localVersion: string;
  vaultUnlock: RemoteVaultUnlockState;
  // engineInfo is the engine information dialog, from which an engine whose
  // version differs from this machine's is updated.
  engineInfo: EngineInfoDialogState;
};

export type EngineInfoDialogState = {
  // openFor is the run target the dialog is open for.
  openFor: string | null;
  ask: (alias: string) => void;
  close: () => void;
};

// useRunTargets holds the run targets: this machine and the engines on SSH
// hosts it forwards terminals to. The choice is remembered by this browser.
export function useRunTargets({
  translate,
  enabled,
  localVersion,
  api = remoteEnginesApi,
}: {
  translate: Translate;
  enabled: boolean;
  localVersion: string;
  api?: RemoteEnginesApi;
}): RunTargetsState {
  const [engines, setEngines] = useState<string[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [stored, setStored] = useState(() => readStoredValue(localStorageKeys.runTarget) ?? "");
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const vaultUnlock = useRemoteVaultUnlock();
  const [engineInfoFor, setEngineInfoFor] = useState<string | null>(null);
  const closeEngineInfo = useCallback(() => setEngineInfoFor(null), []);
  const { allowOffer: allowVaultOffer } = vaultUnlock;

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void api.list()
      .then((listed) => { if (active) setEngines(listed.engines.map((engine) => engine.alias)); })
      .catch(() => undefined)
      .finally(() => { if (active) setLoaded(true); });
    return () => { active = false; };
  }, [api, enabled]);

  // A remembered engine that is no longer a run target falls back to this
  // machine once the list is known.
  const selected = stored !== "" && (!loaded || engines.includes(stored)) ? stored : null;

  const select = useCallback((alias: string | null) => {
    setProblem("");
    // Choosing a run target again asks for its Vault password again, even if
    // that was cancelled before.
    if (alias !== null) allowVaultOffer(alias);
    setStored(alias ?? "");
    writeStoredValue(localStorageKeys.runTarget, alias ?? "");
  }, [allowVaultOffer]);

  // report shows why a request about an engine failed. A different version is
  // answered with the information dialog, whose Update fixes it.
  const report = useCallback((error: unknown, alias: string) => {
    if (failureCode(error) === "remote_engine_incompatible") setEngineInfoFor(alias);
    else setProblem(runTargetProblemText(translate, error, alias));
  }, [translate]);

  const add = useCallback(async (alias: string) => {
    setProblem("");
    setBusy(true);
    try {
      await api.add(alias);
      setEngines((current) => current.includes(alias) ? current : [...current, alias]);
      select(alias);
      return true;
    } catch (error) {
      report(error, alias);
      return false;
    } finally {
      setBusy(false);
    }
  }, [api, report, select]);

  const remove = useCallback(async (alias: string) => {
    setProblem("");
    try {
      const listed = await api.remove(alias);
      setEngines(listed.engines.map((engine) => engine.alias));
      if (stored === alias) select(null);
    } catch (error) {
      report(error, alias);
    }
  }, [api, report, select, stored]);

  const openInNewTab = useCallback(async (alias: string) => {
    setProblem("");
    // The tab is opened before the request so that the browser does not treat
    // it as a pop-up; the URL arrives when the relay is ready.
    const tab = window.open("", "_blank");
    try {
      const entrance = await api.open(alias);
      if (tab === null) window.location.assign(entrance.url);
      else tab.location.href = entrance.url;
    } catch (error) {
      tab?.close();
      report(error, alias);
    }
  }, [api, report]);

  return { engines, selected, problem, busy, select, add, remove, openInNewTab, api, localVersion, vaultUnlock,
    engineInfo: { openFor: engineInfoFor, ask: setEngineInfoFor, close: closeEngineInfo },
  };
}

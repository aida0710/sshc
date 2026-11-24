import { useCallback, useEffect, useState } from "react";
import { remoteEnginesApi, type RemoteEnginesApi } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { keepMachineState } from "../settings/machineState";
import { interfaceState, useSettings } from "../settings/settingsStore";
import { failureCode } from "../api/client";
import { runTargetProblemText } from "./runTargetProblem";
import { useRemoteVaultUnlock, type RemoteVaultUnlockState } from "./useRemoteVaultUnlock";
import { useRunTargetUnread } from "./useRunTargetUnread";

export type RunTargetsState = {
  // engines are the SSH hosts whose engine was added as a run target.
  engines: string[];
  // selected は、画面が使う sshc エンジン（Snippets・設定・同期・更新のほかの画面）。null はこのマシン。
  selected: string | null;
  // problem は、足す・外す・別タブで開くが失敗した理由。次の操作まで残るので、dismissProblem で消せる。
  problem: string;
  dismissProblem: () => void;
  busy: boolean;
  // select は、画面が使う sshc エンジンを替える。保存していない下書きがあって、それを残すと決めたときは替えずに
  // false を返す（同じものを選んだときは true）。
  select: (alias: string | null) => boolean;
  add: (alias: string) => Promise<boolean>;
  remove: (alias: string) => Promise<void>;
  openInNewTab: (alias: string) => Promise<void>;
  // api and localVersion are what the engine information dialog asks with.
  api: RemoteEnginesApi;
  localVersion: string;
  vaultUnlock: RemoteVaultUnlockState;
  // unread are the run targets other than the selected one whose terminals sent a notification since
  // this browser last looked (useRunTargetUnread).
  unread: ReadonlySet<string>;
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

// useRunTargets は、リモートセッションに選べる sshc エンジンを持つ。このマシンと、このマシンのエンジンが
// 画面の操作を取り次ぐ SSH の接続先のエンジンである。選んだものはこのマシンの状態として残る。
export function useRunTargets({
  translate,
  enabled,
  localVersion,
  mayChange = () => true,
  api = remoteEnginesApi,
}: {
  translate: Translate;
  enabled: boolean;
  localVersion: string;
  // mayChange says whether the engine the screens work with may change now. The screens of an engine are made anew
  // for another one, so a screen holding an unsaved draft says no, and asks the person about the draft.
  mayChange?: () => boolean;
  api?: RemoteEnginesApi;
}): RunTargetsState {
  const [engines, setEngines] = useState<string[]>([]);
  const [loaded, setLoaded] = useState(false);
  // The run target follows this machine's state, which choosing one here updates at once.
  const stored = interfaceState(useSettings().document, "runTarget");
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const vaultUnlock = useRemoteVaultUnlock();
  const [engineInfoFor, setEngineInfoFor] = useState<string | null>(null);
  const closeEngineInfo = useCallback(() => setEngineInfoFor(null), []);

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
  const unread = useRunTargetUnread({ api, selected, enabled: enabled && engines.length > 0 });

  const select = useCallback((alias: string | null) => {
    if (alias !== selected && !mayChange()) return false;
    setProblem("");
    keepMachineState("runTarget", alias ?? "");
    return true;
  }, [mayChange, selected]);

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

  const dismissProblem = useCallback(() => setProblem(""), []);
  return { engines, selected, problem, dismissProblem, busy, select, add, remove, openInNewTab, api, localVersion, vaultUnlock, unread,
    engineInfo: { openFor: engineInfoFor, ask: setEngineInfoFor, close: closeEngineInfo },
  };
}

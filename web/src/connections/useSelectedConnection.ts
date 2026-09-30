import { useEffect, useState } from "react";
import type { Problem } from "../api/client";
import { configApi, type HostDetail, type Overview } from "../api/config";
import { toProblem } from "../api/guards";
import { useTranslate } from "../i18n/context";
import { keysApi } from "../keys/api";
import type { HostSelection } from "./ConnectionTree";
import {
  loadConnectionSavedState,
  savedResourcesConfirmed,
  type ConnectionSavedState,
} from "./connectionSavedState";
import type { RefreshState } from "./pageState";
import { connectionSecretsApi } from "./secretsApi";

type SelectedConnectionOptions = {
  selection: HostSelection | null;
  isCurrentSelection: (identity: HostSelection) => boolean;
  onOverviewLoaded: (overview: Overview) => void;
  setProblem: (problem: Problem | null) => void;
  setLocalError: (message: string) => void;
};

// useSelectedConnection は、選んでいる接続の保存済みの内容（設定ファイルの中身と、鍵・Vault・
// 認証情報の状態）を読み込み、保存のあとに読み直す。読み終えたときに別の接続へ移っていたら、
// 読んだ結果は捨てる。
export function useSelectedConnection({
  selection,
  isCurrentSelection,
  onOverviewLoaded,
  setProblem,
  setLocalError,
}: SelectedConnectionOptions) {
  const t = useTranslate();
  const [detail, setDetail] = useState<HostDetail | null>(null);
  const [savedState, setSavedState] = useState<ConnectionSavedState | null>(null);
  const [missingSelection, setMissingSelection] = useState(false);
  const [refreshState, setRefreshState] = useState<RefreshState>("idle");
  const [savedRevision, setSavedRevision] = useState(0);

  const selectedPath = selection === null ? "" : selection.path;
  const selectedAlias = selection === null ? "" : selection.alias;
  useEffect(() => {
    if (selectedAlias === "") return;
    let active = true;
    void configApi
      .host(selectedPath, selectedAlias)
      .then(async (loaded) => ({
        detail: loaded,
        saved: await loadConnectionSavedState(loaded, keysApi, connectionSecretsApi),
      }))
      .then(({ detail: loaded, saved }) => {
        if (active) {
          setDetail(loaded);
          setSavedState(saved);
          setRefreshState("idle");
          setProblem(null);
          setMissingSelection(false);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setDetail(null);
          setSavedState(null);
          setProblem(toProblem(error));
          setMissingSelection(true);
        }
      });
    return () => {
      active = false;
    };
  }, [selectedPath, selectedAlias, setProblem]);

  // forget は、読み込んだ内容を捨てる。同じ接続の識別子が変わったときに使う。
  function forget() {
    setDetail(null);
    setSavedState(null);
    setMissingSelection(false);
  }

  // reset は、別の接続へ移るときに、読み込んだ内容と読み直しの状態を戻す。
  function reset() {
    forget();
    setRefreshState("idle");
  }

  // reloadAfterSave は、保存する前に選んでいた接続を読み直す。読み直すあいだに別の接続へ
  // 移っていたら、結果も失敗も見せない。
  async function reloadAfterSave(identity: HostSelection) {
    try {
      const loaded = await configApi.host(identity.path, identity.alias);
      const saved = await loadConnectionSavedState(loaded, keysApi, connectionSecretsApi);
      if (!isCurrentSelection(identity)) return;
      setDetail(loaded);
      setSavedState(saved);
      setSavedRevision((current) => current + 1);
    } catch (error) {
      if (!isCurrentSelection(identity)) return;
      setProblem(toProblem(error));
      setMissingSelection(true);
    }
  }

  // refreshCommittedConnection は、下書きを書き込んだあとに、一覧と選んでいる接続を読み直す。
  // 鍵・Vault・認証情報のどれかを読めなければ失敗として扱い、接続と編集を止めたままにする。
  async function refreshCommittedConnection() {
    if (selection === null) return;
    const identity = selection;
    setRefreshState("refreshing");
    try {
      const [nextOverview, nextDetail] = await Promise.all([
        configApi.overview(),
        configApi.host(identity.path, identity.alias),
      ]);
      const nextSaved = await loadConnectionSavedState(nextDetail, keysApi, connectionSecretsApi);
      if (!savedResourcesConfirmed(nextSaved)) throw new Error("saved_state_refresh_failed");
      if (!isCurrentSelection(identity)) return;
      onOverviewLoaded(nextOverview);
      setDetail(nextDetail);
      setSavedState(nextSaved);
      setSavedRevision((current) => current + 1);
      setRefreshState("idle");
      setLocalError("");
      setProblem(null);
    } catch {
      if (!isCurrentSelection(identity)) return;
      setRefreshState("failed");
      setLocalError(t("conn.connectionRefreshFailed"));
    }
  }

  return {
    detail,
    savedState,
    missingSelection,
    refreshState,
    savedRevision,
    forget,
    reset,
    reloadAfterSave,
    refreshCommittedConnection,
  };
}

export type SelectedConnection = ReturnType<typeof useSelectedConnection>;

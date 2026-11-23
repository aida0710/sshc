import { useEffect, useMemo } from "react";
import { configApi, createConfigApi, type Overview } from "../api/config";
import { createReadScope } from "../api/readScope";
import { createRecentConnectionsApi, recentConnectionsApi, type RecentConnectionList } from "../api/recentConnections";
import { engineApiBase } from "../api/remoteEngines";

export type HomeApis = {
  loadOverview: () => Promise<Overview>;
  loadRecent: () => Promise<RecentConnectionList>;
};

const thisMachine: HomeApis = {
  loadOverview: () => configApi.overview(),
  loadRecent: () => recentConnectionsApi.recentConnections(),
};

// useHomeApis は、Home が接続先の一覧と最近の接続を読む相手を返す。リモートセッションを選んでいる間は、
// その sshc エンジンのものを、このマシンのエンジンの取り次ぎで読む。セッションを開くのもそのエンジンなので、
// 一覧に出る接続名は、開くときに使える名前と同じになる。
// 実行先を離れるときは、まだ答えを待っている読み取りを終わらせる（useRunTargetApis と同じ理由）。
export function useHomeApis(runTarget: string | null): HomeApis {
  const apis = useMemo(() => {
    if (runTarget === null) return { ...thisMachine, reads: null };
    const reads = createReadScope();
    const base = engineApiBase(runTarget);
    const config = createConfigApi(base, reads);
    const recent = createRecentConnectionsApi(base, reads);
    return { loadOverview: () => config.overview(), loadRecent: () => recent.recentConnections(), reads };
  }, [runTarget]);
  useEffect(() => () => apis.reads?.end(), [apis]);
  return apis;
}

import { createContext, useContext, useEffect, useMemo, type ReactNode } from "react";
import { configApi, createConfigApi, type ConfigApi } from "../api/config";
import { createCredentialsApi, credentialsApi, type CredentialsApi } from "../api/credentials";
import { createDiagnosticsApi, diagnosticsApi, type DiagnosticsApi } from "../api/diagnostics";
import { createVaultApi, vaultApi, type VaultApi } from "../api/vault";
import { createVPNApi, vpnApi, type VPNApi } from "../api/vpn";
import { createKeysApi, keysApi, type KeysApi } from "../keys/api";
import { createRemoteKeysApi, remoteKeysApi, type RemoteKeysApi } from "../remotekeys/api";
import { createReadScope, type ReadScope } from "../api/readScope";
import { createRecentConnectionsApi, recentConnectionsApi, type RecentConnectionsApi } from "../api/recentConnections";
import { engineApiBase } from "../api/remoteEngines";

// EngineApis は、画面が相手にする sshc エンジンの API である。リモートセッションを選んでいる間は、その実行先の
// ものを、このマシンのエンジンの取り次ぎで呼ぶ。設定・同期・更新・スニペットはどのエンジンを選んでいても
// このマシンのものなので、ここには入れない（それぞれの API をそのまま使う）。
export type EngineApis = {
  // runTarget は相手のエンジン。null はこのマシン。
  runTarget: string | null;
  config: ConfigApi;
  recentConnections: RecentConnectionsApi;
  keys: KeysApi;
  remoteKeys: RemoteKeysApi;
  vpn: VPNApi;
  diagnostics: DiagnosticsApi;
  // secrets は、そのエンジンの Vault と、Vault に入れた値の API である。
  secrets: VaultApi & CredentialsApi;
  // hostDetail は、接続先の詳細が使う、診断と Vault の API である。
  hostDetail: DiagnosticsApi & VaultApi & CredentialsApi;
};

function engineApisAt(runTarget: string | null, parts: Omit<EngineApis, "runTarget" | "secrets" | "hostDetail"> & { vault: VaultApi; credentials: CredentialsApi }): EngineApis {
  const { vault, credentials, ...apis } = parts;
  const secrets = { ...credentials, ...vault };
  return { runTarget, ...apis, secrets, hostDetail: { ...apis.diagnostics, ...secrets } };
}

const thisMachine = engineApisAt(null, {
  config: configApi, recentConnections: recentConnectionsApi, keys: keysApi, remoteKeys: remoteKeysApi, vpn: vpnApi,
  diagnostics: diagnosticsApi, vault: vaultApi, credentials: credentialsApi,
});

const EngineApisContext = createContext<EngineApis>(thisMachine);

// useEngineApisOf は、runTarget の API を作る。全体で1つの宛先を切り替えるのではなく、部品の木に渡す。
// 実行先を替える前に始めた操作は、替えたあとも、始めたときのエンジンへ続きを送る。
// 実行先を離れるときは、まだ答えを待っている読み取りを終わらせる（useRunTargetApis と同じ理由）。
//
// 実行先の Vault のロックを解除したら、API を作り直す（vaultRevision）。ロック中に読めなかった画面が、
// 新しい API を見て読み直す。
export function useEngineApisOf(runTarget: string | null, vaultRevision = 0): EngineApis {
  const built = useMemo((): { apis: EngineApis; reads: ReadScope | null } => {
    if (runTarget === null) return { apis: thisMachine, reads: null };
    const reads = createReadScope();
    const base = engineApiBase(runTarget);
    const apis = engineApisAt(runTarget, {
      config: createConfigApi(base, reads), recentConnections: createRecentConnectionsApi(base, reads),
      keys: createKeysApi(base), remoteKeys: createRemoteKeysApi(base), vpn: createVPNApi(base),
      diagnostics: createDiagnosticsApi(base), vault: createVaultApi(base), credentials: createCredentialsApi(base),
    });
    return { apis, reads };
    // vaultRevision は、作り直すきっかけとしてだけ使う。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runTarget, vaultRevision]);
  useEffect(() => () => built.reads?.end(), [built]);
  return built.apis;
}

export function EngineApisProvider({ apis, children }: { apis: EngineApis; children: ReactNode }) {
  return <EngineApisContext.Provider value={apis}>{children}</EngineApisContext.Provider>;
}

// useEngineApis は、この部品の木が相手にする sshc エンジンの API を返す。Provider の外（部品だけを描く
// 単体テスト）では、このマシンのものを返す。
export function useEngineApis(): EngineApis {
  return useContext(EngineApisContext);
}

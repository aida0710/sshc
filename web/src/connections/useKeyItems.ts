import { useEngineApis } from "../shell/engineApis";
import { useEffect, useState } from "react";
import type { KeyItem, KeysApi } from "../keys/api";

export type KeyItemsState = { kind: "loading" } | { kind: "ready"; items: KeyItem[] } | { kind: "failed" };

// useKeyItems は、~/.ssh の鍵の一覧を1回読む。グループの欄が keys/<グループ>/ の中身を出すのに使う。
export function useKeyItems(apiOverride?: Pick<KeysApi, "inventory">): KeyItemsState {
  const engineApis = useEngineApis();
  const api = apiOverride ?? engineApis.keys;
  const [state, setState] = useState<KeyItemsState>({ kind: "loading" });
  useEffect(() => {
    let active = true;
    api.inventory().then(
      (inventory) => { if (active) setState({ kind: "ready", items: inventory.items }); },
      () => { if (active) setState({ kind: "failed" }); },
    );
    return () => {
      active = false;
    };
  }, [api]);
  return state;
}

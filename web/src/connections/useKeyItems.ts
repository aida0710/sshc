import { useEffect, useState } from "react";
import { keysApi, type KeyItem, type KeysApi } from "../keys/api";

export type KeyItemsState = { kind: "loading" } | { kind: "ready"; items: KeyItem[] } | { kind: "failed" };

// useKeyItems は、~/.ssh の鍵の一覧を1回読む。グループの欄が keys/<グループ>/ の中身を出すのに使う。
export function useKeyItems(api: Pick<KeysApi, "inventory"> = keysApi): KeyItemsState {
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

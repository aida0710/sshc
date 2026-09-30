import type { KeyItem, KeysApi } from "./api";
import { PublicKeyPanel } from "./PublicKeyPanel";
import { RevealDialog } from "./RevealDialog";

// KeyRowPanelState は、鍵の行の下に開くパネルである。秘密鍵を表示する前の確認と、
// 読み出した公開鍵のどちらか1つだけを開く。
export type KeyRowPanelState =
  | { kind: "reveal"; item: KeyItem }
  | { kind: "publicKey"; item: KeyItem; relativePath: string; text: string };

export function KeyRowPanel({
  panel,
  api,
  onClose,
}: {
  panel: KeyRowPanelState;
  api: KeysApi;
  onClose: () => void;
}) {
  if (panel.kind === "reveal") {
    return <RevealDialog keyId={panel.item.id} relativePath={panel.item.relativePath} api={api} onClose={onClose} />;
  }
  return <PublicKeyPanel relativePath={panel.relativePath} text={panel.text} onClose={onClose} />;
}

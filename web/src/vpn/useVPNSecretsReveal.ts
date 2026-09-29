import { useEffect, useEffectEvent, useState } from "react";
import type { VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { describeVPNProblem } from "./vpnProblemMessage";

// VPN プロファイルの編集を開いたときに、保存済みのシークレットを engine から一度だけ取り出す。

// SecretsReveal は、取り出した結果である。
export type SecretsReveal =
  | { state: "revealing" }
  | { state: "revealed"; secrets: VPNSecrets }
  // reason は、取り出せなかった理由の1文である（Vault がロックされている、など）。
  | { state: "failed"; reason: string };

export function useVPNSecretsReveal({
  name,
  revealSecrets,
  onRevealed,
}: {
  // name は、編集するプロファイルの名前である。新しく作るフォームでは無い。
  name: string | undefined;
  revealSecrets: ((name: string) => Promise<VPNSecrets>) | undefined;
  // onRevealed は、取り出したシークレットをフォームの欄に入れる。
  onRevealed: (secrets: VPNSecrets) => void;
}): { reveal: SecretsReveal | null; forget: () => void } {
  const t = useTranslate();
  const [reveal, setReveal] = useState<SecretsReveal | null>(
    () => (name === undefined || revealSecrets === undefined ? null : { state: "revealing" }),
  );
  const revealed = useEffectEvent((secrets: VPNSecrets) => {
    setReveal({ state: "revealed", secrets });
    onRevealed(secrets);
  });
  const failed = useEffectEvent((caught: unknown) => {
    setReveal({ state: "failed", reason: describeVPNProblem(t, caught) ?? t("vpn.failed") });
  });

  // 一覧は定期的に読み直され、編集中のプロファイルも新しいオブジェクトになる。依存には名前だけを
  // 入れ、読み直しのたびに取り出し直さない。
  useEffect(() => {
    if (name === undefined || revealSecrets === undefined) return;
    let active = true;
    revealSecrets(name).then(
      (secrets) => {
        if (active) revealed(secrets);
      },
      (caught: unknown) => {
        if (active) failed(caught);
      },
    );
    return () => {
      active = false;
    };
  }, [name, revealSecrets]);

  // forget は、取り出したシークレットを、この状態にも残さない。編集を閉じるときに呼ぶ。
  return { reveal, forget: () => setReveal(null) };
}

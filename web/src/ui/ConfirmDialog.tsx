import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { dangerAction, secondaryAction } from "./form";

// **問いを挟んでよいのは、押し戻せない操作だけである。**
//
// この規則そのものは正しく、この repo は概ねそれで通っている——接続の削除は
// 履歴から戻せるので訊かず、鍵は trash へ入るので訊かず、trash から消す
// （purge）はサーバが確認のトークンを要求する。
//
// **間違えやすいのは「戻せる」の判定である。** 走っている接続を閉じるのは、
// かつて「開き直せば済む」に分類されていた。開き直して得られるのは同じ相手への
// *新しい* セッションであり、動いていたもの——編集中のファイル、走っている
// ビルド、追っているログ——は戻らない。**開き直しは、取り消しではない。**
//
// ここに置いたのは、そう判定した数少ない場所で同じ姿を出すためである。

/**
 * ConfirmDialog は、取り返しのつかない操作の前に一度だけ訊く。
 *
 * **開いた時点で focus は「やめる」側に居る。** 何も読まずに Enter を叩いた人
 * が落ちる先は、失うものが無い方でなければならない。Escape も同じ側へ落ちる。
 *
 * **body へ出す。** `fixed` は窓を基準にする——**ただし祖先が transform を
 * 持っていないときだけである。** ナビゲーションの板は開閉のために常に
 * translate を持ち、さらに overflow-hidden で切る。あの中に置かれた確認は
 * 幅 288px の板に閉じ込められ、文も釦も見切れていた。
 *
 * 出し先を親に決めさせない。**問いは、それを出した場所の都合とは無関係に
 * 読めなければならない。**
 */
export function ConfirmDialog({
  id,
  heading,
  body,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
}: {
  id: string;
  heading: string;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    cancelRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onCancel]);

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/75 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
        className="flex w-full max-w-sm flex-col gap-3 rounded-lg border border-control-line bg-card p-4"
      >
        <h2 id={id} className="text-sm font-medium text-ink">
          {heading}
        </h2>
        {body}
        <div className="flex justify-end gap-2">
          <button ref={cancelRef} type="button" onClick={onCancel} className={secondaryAction}>
            {cancelLabel}
          </button>
          <button type="button" onClick={onConfirm} className={dangerAction}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { ModalShell } from "./ModalShell";
import { Button } from "./surface";

// ConfirmDialog は、取り消せない操作の前に確かめる。
//
// 確かめた操作の失敗をどこに出すかは、呼び手が次のどちらかに決める。
// - 閉じずに待つ: onConfirm から Promise を返し、失敗の文を error で渡す。失敗は
//   このダイアログの中に出る（背面に出すとダイアログに隠れ、利用者は気づかずに
//   繰り返す）。成功したら呼び手がダイアログを閉じる。設定ファイル・鍵のゴミ箱・
//   背景画像・SFTP・known_hosts の削除がこちら。
// - 先に閉じる: onConfirm の中でダイアログを閉じてから操作を始め、失敗は背面の画面に
//   出す。ダイアログが残らないので、失敗が隠れない。接続・シークレット・VPN
//   プロファイル・スニペットの削除などがこちら。
// onConfirm が Promise を返す間と busy の間は、両方のボタンとダイアログを閉じる操作を
// 止め、二度押しで要求が 2 回飛ばないようにする。
// confirmDisabled は、確かめる内容がまだ揃っていない間（確認の前提をサーバーから
// 読み込んでいる間など）、確認ボタンだけを止める。キャンセルとダイアログを閉じる
// 操作は止めない。開いた直後のフォーカスもキャンセルに置いたままにする。
export function ConfirmDialog({
  id,
  heading,
  body,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
  returnFocusRef,
  confirmKind = "danger",
  busy = false,
  confirmDisabled = false,
  error = "",
}: {
  id: string;
  heading: string;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  onConfirm: () => void | Promise<unknown>;
  onCancel: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
  confirmKind?: "primary" | "danger";
  busy?: boolean;
  confirmDisabled?: boolean;
  error?: string;
}) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);
  const [confirming, setConfirming] = useState(false);
  const locked = busy || confirming;
  const bodyId = `${id}-body`;
  const errorId = `${id}-error`;
  useFocusConfirmAfterUnlock(locked, confirmRef);

  function confirm() {
    if (locked || confirmDisabled) return;
    const pending = onConfirm();
    if (pending === undefined) return;
    setConfirming(true);
    // 失敗の文は呼び手が error で渡す。ここでは終わるのを待つだけにする。
    void pending.catch(() => undefined).finally(() => setConfirming(false));
  }

  return (
    <ModalShell
      labelledBy={id}
      describedBy={error === "" ? bodyId : `${bodyId} ${errorId}`}
      onDismiss={onCancel}
      dismissible={!locked}
      initialFocusRef={cancelRef}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="flex w-full max-w-sm flex-col gap-3 rounded-lg p-4"
    >
      <h2 id={id} className="text-sm font-medium text-ink">
        {heading}
      </h2>
      <div id={bodyId} className="flex flex-col gap-3">{body}</div>
      {error === "" ? null : (
        <p id={errorId} role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button ref={cancelRef} disabled={locked} onClick={onCancel} className="focus:outline-2 focus:outline-offset-2 focus:outline-accent">
          {cancelLabel}
        </Button>
        <Button ref={confirmRef} kind={confirmKind} disabled={locked || confirmDisabled} onClick={confirm} className="focus:outline-2 focus:outline-offset-2 focus:outline-accent">
          {confirmLabel}
        </Button>
      </div>
    </ModalShell>
  );
}

// 処理中は両方のボタンを無効にするので、押した確認ボタンからフォーカスが外れ、ブラウザ
// によっては body へ落ちる。失敗してダイアログが開いたままなら、フォーカスを確認ボタンへ
// 戻し、キーボードでそのままやり直せるようにする。成功したら呼び手が閉じるので何もしない。
function useFocusConfirmAfterUnlock(locked: boolean, confirmRef: RefObject<HTMLButtonElement | null>) {
  const wasLocked = useRef(false);
  useEffect(() => {
    if (locked) {
      wasLocked.current = true;
      return;
    }
    if (!wasLocked.current) return;
    wasLocked.current = false;
    const confirmButton = confirmRef.current;
    if (confirmButton === null) return;
    if (confirmButton.closest('[role="dialog"]')?.contains(document.activeElement) === true) return;
    confirmButton.focus();
  }, [locked, confirmRef]);
}

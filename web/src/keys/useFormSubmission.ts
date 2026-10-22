import { useState } from "react";

// useFormSubmission は、Keys の画面のフォームが送った操作を待つ間 submitting を立てる。
// 送信のボタンはこれで回転の印を出し、待っている間の二度目の送信（入力欄の Enter による
// 送信を含む）は送らない。失敗は画面の一つの通知が出すので、ここでは終わるのを待つだけにする。
// 画面全体の処理中ではなくフォームごとに持つのは、ほかのフォームの操作の最中に、押していない
// 送信のボタンが回らないようにするためである。
export function useFormSubmission<Args extends unknown[]>(send: (...args: Args) => Promise<unknown>) {
  const [submitting, setSubmitting] = useState(false);
  async function submit(...args: Args) {
    if (submitting) return;
    setSubmitting(true);
    try {
      await send(...args);
    } finally {
      setSubmitting(false);
    }
  }
  return { submitting, submit };
}

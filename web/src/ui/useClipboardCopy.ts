import { useEffect, useState } from "react";
import { clipboard } from "./clipboard";

// コピーの結果。押した直後にその場で見せる。
export type CopyState = "idle" | "copied" | "failed";

// copiedVisibleMs は、押したボタンに ✓ を出しておく長さ。結果を見るには足り、続けてもう一度押すときには
// 元の印に戻っている長さにする（2026-11-25、コピーの知らせ方を「押した所で ✓、閉じる所はトースト」にそろえた）。
export const copiedVisibleMs = 2000;

// useClipboardCopy は、値をクリップボードへ書き、その結果を表示用に保つ。
//
// 表示している値が変わったら idle へ戻す。手元のクリップボードにあるのは前の値
// なので、新しい値の隣で「コピーしました」と言い続けると嘘になる。書けたことは copiedVisibleMs のあと
// idle へ戻す。断られたことは、次に押すか値が変わるまで残す。
export function useClipboardCopy(value: string): { state: CopyState; copy: () => void } {
  const [state, setState] = useState<CopyState>("idle");
  // copies は書けた回数。続けて押したときも、✓ を出す長さを数え直すために使う。
  const [copies, setCopies] = useState(0);

  useEffect(() => {
    setState("idle");
  }, [value]);

  useEffect(() => {
    if (state !== "copied") return;
    const timer = window.setTimeout(() => setState("idle"), copiedVisibleMs);
    return () => window.clearTimeout(timer);
  }, [state, copies]);

  async function write() {
    try {
      await clipboard.writeText(value);
      setState("copied");
      setCopies((count) => count + 1);
    } catch {
      setState("failed");
    }
  }

  return { state, copy: () => void write() };
}

import { useEffect, useState } from "react";
import { clipboard } from "./clipboard";

// コピーの結果。押した直後にその場で見せる。
export type CopyState = "idle" | "copied" | "failed";

// useClipboardCopy は、値をクリップボードへ書き、その結果を表示用に保つ。
//
// 表示している値が変わったら idle へ戻す。手元のクリップボードにあるのは前の値
// なので、新しい値の隣で「コピーしました」と言い続けると嘘になる。
export function useClipboardCopy(value: string): { state: CopyState; copy: () => void } {
  const [state, setState] = useState<CopyState>("idle");

  useEffect(() => {
    setState("idle");
  }, [value]);

  async function write() {
    try {
      await clipboard.writeText(value);
      setState("copied");
    } catch {
      setState("failed");
    }
  }

  return { state, copy: () => void write() };
}

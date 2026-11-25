import { useEffect } from "react";
import { useToast } from "../ui/useToast";

// useSessionProblemToast は、セッションの操作（開く・閉じる・つなぎ直す）が失敗した理由を、出たときに知らせとしても出す。
// 理由はサイドバーに残るが、スマホではサイドバーが隠れていて、PC でも隠せる。どの画面にいても気づけるようにする。
export function useSessionProblemToast(problem: string): void {
  const showToast = useToast();
  useEffect(() => {
    if (problem !== "") showToast(problem);
  }, [problem, showToast]);
}

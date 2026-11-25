import { useState } from "react";

// HostKeyAnswer answers the question a connection asks about an unknown host key, in the terminal or an SFTP tab. In
// the terminal the strip above and the card over it both offer it, so they share one state (a press on either shows as
// busy on both).
export type HostKeyAnswer = {
  // answer is undefined when this view cannot answer from a button.
  answer: ((fingerprint: string, accept: boolean) => void) | undefined;
  // answering は、送っている答え（true は［接続する］、false は［やめる］）。押したほうのボタンだけに回る印を出し、
  // もう一方は押せなくする。答えを送っていないときは null。
  answering: boolean | null;
};

export function useHostKeyAnswer(onAnswerHostKey: ((fingerprint: string, accept: boolean) => Promise<boolean>) | undefined): HostKeyAnswer {
  const [answering, setAnswering] = useState<boolean | null>(null);

  async function answer(handler: (fingerprint: string, accept: boolean) => Promise<boolean>, fingerprint: string, accept: boolean) {
    if (answering !== null) return;
    setAnswering(accept);
    try {
      await handler(fingerprint, accept);
    } finally {
      setAnswering(null);
    }
  }

  return {
    answer: onAnswerHostKey === undefined ? undefined : (fingerprint, accept) => void answer(onAnswerHostKey, fingerprint, accept),
    answering,
  };
}

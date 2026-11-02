import { useState } from "react";

// HostKeyAnswer answers the question a connecting session asks about an unknown host key. The strip above the
// terminal and the card over it both offer it, so they share one state (a press on either shows as busy on both).
export type HostKeyAnswer = {
  // answer is undefined when this view cannot answer from a button.
  answer: ((fingerprint: string, accept: boolean) => void) | undefined;
  busy: boolean;
};

export function useHostKeyAnswer(onAnswerHostKey: ((fingerprint: string, accept: boolean) => Promise<boolean>) | undefined): HostKeyAnswer {
  const [busy, setBusy] = useState(false);

  async function answer(handler: (fingerprint: string, accept: boolean) => Promise<boolean>, fingerprint: string, accept: boolean) {
    if (busy) return;
    setBusy(true);
    try {
      await handler(fingerprint, accept);
    } finally {
      setBusy(false);
    }
  }

  return {
    answer: onAnswerHostKey === undefined ? undefined : (fingerprint, accept) => void answer(onAnswerHostKey, fingerprint, accept),
    busy,
  };
}

import { useCallback, useState } from "react";
import { useTranslate } from "../i18n/context";
import { transferControlFailureMessage } from "./transferControlFailure";
import { sftpTransferManager } from "./transferManager";

export type TransferControl = {
  // The message for the last change that did not go through, or "".
  problem: string;
  runControl: (operation: () => Promise<void>) => void;
  dismissProblem: () => void;
};

// Runs changes to the engine's transfer queue and its settings. A change that
// fails lists the queue again, so the screen shows what the engine holds.
export function useTransferControl(): TransferControl {
  const t = useTranslate();
  const [problem, setProblem] = useState("");
  const runControl = useCallback((operation: () => Promise<void>) => {
    setProblem("");
    void operation().catch(async (error: unknown) => {
      await sftpTransferManager.reconcile().catch(() => undefined);
      setProblem(t(transferControlFailureMessage(error)));
    });
  }, [t]);
  const dismissProblem = useCallback(() => setProblem(""), []);
  return { problem, runControl, dismissProblem };
}

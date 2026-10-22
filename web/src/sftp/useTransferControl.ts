import { useCallback, useState } from "react";
import { useTranslate } from "../i18n/context";
import { transferControlFailureMessage } from "./transferControlFailure";
import { sftpTransferManager } from "./transferManager";

// runControl resolves to whether the change went through. A change that did not
// go through has already been told to the user through `problem`, so the caller
// only decides what to show while it waits and after it succeeds.
export type RunTransferControl = (operation: () => Promise<void>) => Promise<boolean>;

export type TransferControl = {
  // The message for the last change that did not go through, or "".
  problem: string;
  runControl: RunTransferControl;
  dismissProblem: () => void;
};

// Runs changes to the engine's transfer queue and its settings. A change that
// fails lists the queue again, so the screen shows what the engine holds.
export function useTransferControl(): TransferControl {
  const t = useTranslate();
  const [problem, setProblem] = useState("");
  const runControl = useCallback<RunTransferControl>(async (operation) => {
    setProblem("");
    try {
      await operation();
      return true;
    } catch (error) {
      await sftpTransferManager.reconcile().catch(() => undefined);
      setProblem(t(transferControlFailureMessage(error)));
      return false;
    }
  }, [t]);
  const dismissProblem = useCallback(() => setProblem(""), []);
  return { problem, runControl, dismissProblem };
}

import { useCallback } from "react";
import { useTranslate, type Values } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { useAsyncOperation } from "../ui/useAsyncOperation";

// KeyOperationRefused is a failure the Keys screen can already name: the
// engine answered but refused, or a precondition did not hold. Its message is
// shown in place of the operation's general failure message.
export class KeyOperationRefused extends Error {
  constructor(
    readonly messageKey: MessageKey,
    readonly values?: Values,
  ) {
    super(messageKey);
    this.name = "KeyOperationRefused";
  }
}

// RunKeyOperation runs one operation of the Keys screen. It clears the last
// operation's failure when it starts and, if the work throws, shows
// failedMessage, or the refusal the work named. Resolves to whether the work
// succeeded.
export type RunKeyOperation = (work: () => Promise<void>, failedMessage: MessageKey) => Promise<boolean>;

// useKeyOperation gives every operation of the Keys screen the one failure
// notice the screen shows, so no operation leaves an earlier failure standing
// after it succeeds.
export function useKeyOperation() {
  const t = useTranslate();
  const { error, fail, clearError, run } = useAsyncOperation();
  const runKeyOperation = useCallback<RunKeyOperation>(
    (work, failedMessage) =>
      run(work, {
        describe: (caught) =>
          caught instanceof KeyOperationRefused ? t(caught.messageKey, caught.values) : t(failedMessage),
      }),
    [run, t],
  );
  return { failure: error, fail, clearFailure: clearError, run: runKeyOperation };
}

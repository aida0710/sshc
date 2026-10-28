import { useCallback, useState } from "react";
import { failureCode, failurePath } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { SyncResultView } from "./SyncResultCard";

type Refusals = Readonly<Partial<Record<string, MessageKey>>>;
type Recover = (code: string) => Promise<boolean>;

// SyncRunOptions is what a Sync operation does with its result and how it explains a refusal.
export type SyncRunOptions<T> = {
  apply: (value: T) => void;
  // failure is the message when no more specific one names the refusal.
  failure: string;
  // explain names a refusal the operation knows better than the shared table.
  explain?: (code: string) => string;
};

type SyncExecuteOptions<T> = SyncRunOptions<T> & {
  // recover handles a refusal itself (for example by reading the state again); true means handled.
  recover?: Recover;
};

export function useSyncOperation(
  t: Translate,
  refusals: Refusals,
  pathRefusals: Refusals,
) {
  const [resultView, setResultView] = useState<SyncResultView | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [errorCode, setErrorCode] = useState("");
  const [busy, setBusy] = useState(false);

  const clearError = useCallback(() => {
    setError("");
    setErrorCode("");
  }, []);

  // engine がファイルを示した失敗は、そのパスを入れた文で説明する。
  function describeRefusal(
    code: string,
    path: string,
    failure: string,
  ): string {
    const namedWithPath = path === "" ? undefined : pathRefusals[code];
    if (namedWithPath !== undefined) return t(namedWithPath, { path });
    const named = refusals[code];
    return named === undefined ? failure : t(named);
  }

  async function execute<T>(operation: () => Promise<T>, options: SyncExecuteOptions<T>) {
    const { apply, failure, explain, recover } = options;
    clearError();
    setNotice("");
    setBusy(true);
    try {
      apply(await operation());
    } catch (caught) {
      const code = failureCode(caught);
      if (recover !== undefined && (await recover(code))) return;
      setErrorCode(code);
      setError(
        explain?.(code) || describeRefusal(code, failurePath(caught), failure),
      );
    } finally {
      setBusy(false);
    }
  }

  return {
    resultView,
    notice,
    error,
    errorCode,
    busy,
    execute,
    clearError,
    showNotice: setNotice,
    showResult: setResultView,
  };
}

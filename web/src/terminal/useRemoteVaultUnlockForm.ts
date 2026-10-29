import { useCallback, useEffect, useState, type FormEvent } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { runTargetProblemText } from "./runTargetProblem";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { useLoad } from "../ui/useLoad";

export type RemoteVaultUnlockForm = {
  password: string;
  setPassword: (password: string) => void;
  busy: boolean;
  // error says why the password was not accepted; empty when it was not tried.
  error: string;
  // vaultMissing is null until the engine says whether it has a Vault. An
  // engine with no Vault yet is refused the same way as a locked one; that
  // Vault is created in the engine's own screen.
  vaultMissing: boolean | null;
  unlock: (event: FormEvent) => Promise<void>;
};

// useRemoteVaultUnlockForm asks the Vault password of a run-target engine and
// unlocks it. The password goes to that engine through this machine's engine;
// neither keeps it here.
export function useRemoteVaultUnlockForm({
  alias,
  api,
  translate,
  onUnlocked,
}: {
  alias: string;
  api: RemoteEnginesApi;
  translate: Translate;
  onUnlocked: () => void;
}): RemoteVaultUnlockForm {
  const [password, setPassword] = useState("");
  const operation = useAsyncOperation();
  const readStatus = useCallback(() => api.status(alias), [alias, api]);
  const status = useLoad(readStatus);
  // Unlocked elsewhere in the meantime: there is nothing to ask.
  const unlockedElsewhere = status.state === "loaded" && status.value.vault && status.value.unlocked;
  useEffect(() => {
    if (unlockedElsewhere) onUnlocked();
  }, [unlockedElsewhere, onUnlocked]);
  // A status that could not be read still asks for the password; unlocking says what is wrong.
  const vaultMissing = status.state === "loading" ? null : status.state === "loaded" && !status.value.vault;

  async function unlock(event: FormEvent) {
    event.preventDefault();
    if (operation.busy || password === "") return;
    await operation.run(() => api.unlockVault(alias, password), {
      apply: () => {
        setPassword("");
        onUnlocked();
      },
      describe: (caught) => runTargetProblemText(translate, caught, alias),
    });
  }

  return { password, setPassword, busy: operation.busy, error: operation.error, vaultMissing, unlock };
}

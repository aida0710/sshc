import { useEffect, useState, type FormEvent } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import type { Translate } from "../i18n/context";
import { runTargetProblemText } from "./runTargetProblem";

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
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [vaultMissing, setVaultMissing] = useState<boolean | null>(null);

  useEffect(() => {
    let active = true;
    void api.status(alias)
      .then((status) => {
        if (!active) return;
        // Unlocked elsewhere in the meantime: there is nothing to ask.
        if (status.vault && status.unlocked) onUnlocked();
        else setVaultMissing(!status.vault);
      })
      // The password is still asked for; unlocking says what is wrong.
      .catch(() => { if (active) setVaultMissing(false); });
    return () => { active = false; };
  }, [alias, api, onUnlocked]);

  async function unlock(event: FormEvent) {
    event.preventDefault();
    if (busy || password === "") return;
    setBusy(true);
    setError("");
    try {
      await api.unlockVault(alias, password);
      setPassword("");
      onUnlocked();
    } catch (caught) {
      setError(runTargetProblemText(translate, caught, alias));
    } finally {
      setBusy(false);
    }
  }

  return { password, setPassword, busy, error, vaultMissing, unlock };
}

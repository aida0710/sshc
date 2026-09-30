import { useCallback } from "react";
import type { Credential, CredentialList } from "../api/credentials";
import type { KeyItem } from "./api";
import { useStoredPhrases } from "./forms";
import type { KeySecretsApi } from "./secretsApi";
import { KeyOperationRefused } from "./useKeyOperation";

function keyPassphrasesIn(listed: CredentialList): Credential[] {
  return listed.credentials.filter((credential) => credential.kind === "key_passphrase");
}

// useKeyPassphrases keeps the stored passphrases the key list can assign and
// runs the vault calls that change them. A call that fails throws; the screen
// runs each call as a key operation and owns the notice and its wording.
export function useKeyPassphrases(secrets: KeySecretsApi) {
  const stored = useStoredPhrases();
  const { setPhrases, setDedicatedPhrasePaths, setChosenPhrase } = stored;

  // A named passphrase assigned to a key replaces the key's dedicated one.
  const applyAssignment = useCallback((listed: CredentialList, item: KeyItem) => {
    setPhrases(keyPassphrasesIn(listed));
    setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
    setChosenPhrase("");
  }, [setChosenPhrase, setDedicatedPhrasePaths, setPhrases]);

  const load = useCallback(async () => {
    try {
      const status = await secrets.passwordVault();
      setDedicatedPhrasePaths(status.dedicatedKeyPassphrases);
      setPhrases(status.unlocked ? keyPassphrasesIn(await secrets.credentials()) : []);
    } catch {
      setPhrases([]);
      setDedicatedPhrasePaths([]);
    }
  }, [secrets, setDedicatedPhrasePaths, setPhrases]);

  async function assign(item: KeyItem, name: string): Promise<void> {
    applyAssignment(await secrets.assignCredential("key_passphrase", item.relativePath, name), item);
  }

  async function storeAndAssign(item: KeyItem, name: string, secret: string): Promise<void> {
    if (stored.phrases.some((credential) => credential.name === name)) {
      throw new KeyOperationRefused("keys.storedPassphraseExists");
    }
    await secrets.storeCredential("key_passphrase", name, secret);
    applyAssignment(await secrets.assignCredential("key_passphrase", item.relativePath, name), item);
  }

  async function unassign(item: KeyItem): Promise<void> {
    applyAssignment(await secrets.unassignCredential("key_passphrase", item.relativePath), item);
  }

  return { ...stored, load, assign, storeAndAssign, unassign };
}

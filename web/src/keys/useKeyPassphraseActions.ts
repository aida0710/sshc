import type { KeyItem, KeysApi } from "./api";
import type { useAgentForm, usePassphraseForm, useStoredPassphraseForm } from "./forms";
import type { useKeyPassphrases } from "./useKeyPassphrases";
import type { RunKeyOperation } from "./useKeyOperation";

type KeyPassphraseActionsOptions = {
  api: KeysApi;
  refresh: () => Promise<void>;
  runKeyOperation: RunKeyOperation;
  storedPhrases: ReturnType<typeof useKeyPassphrases>;
  passphraseForm: ReturnType<typeof usePassphraseForm>;
  agentForm: ReturnType<typeof useAgentForm>;
  storedPassphraseForm: ReturnType<typeof useStoredPassphraseForm>;
};

// useKeyPassphraseActions は、パスフレーズを使う鍵の操作をまとめる。パスフレーズの
// 変更、保存したパスフレーズの割り当てと取り外し、ssh-agent への登録と取り外しである。
// 断られたら、入力したパスフレーズを残さない。
export function useKeyPassphraseActions({
  api,
  refresh,
  runKeyOperation,
  storedPhrases,
  passphraseForm,
  agentForm,
  storedPassphraseForm,
}: KeyPassphraseActionsOptions) {
  async function changePassphrase(item: KeyItem) {
    const { currentPassphrase, newPassphrase, removePassphrase } = passphraseForm;
    const changed = await runKeyOperation(async () => {
      await api.changePassphrase(item.id, {
        currentPassphrase,
        newPassphrase: removePassphrase ? "" : newPassphrase,
        unencrypted: removePassphrase,
      });
      passphraseForm.close();
      await refresh();
    }, "keys.passphraseFailed");
    if (changed) return;
    passphraseForm.setCurrentPassphrase("");
    passphraseForm.setNewPassphrase("");
  }

  async function registerWithAgent(item: KeyItem) {
    const registered = await runKeyOperation(async () => {
      await api.registerWithAgent(item.id, {
        passphrase: agentForm.agentPassphrase,
        lifetimeSeconds: agentForm.agentLifetime,
      });
      agentForm.close();
      await refresh();
    }, "keys.agentFailed");
    if (!registered) agentForm.setAgentPassphrase("");
  }

  async function removeFromAgent(keyId: string) {
    await runKeyOperation(async () => {
      await api.deregisterFromAgent(keyId);
      await refresh();
    }, "keys.agentRemoveFailed");
  }

  async function assignPhrase(item: KeyItem) {
    await runKeyOperation(
      () => storedPhrases.assign(item, storedPhrases.chosenPhrase),
      "keys.assignPassphraseFailed",
    );
  }

  async function storeAndAssignPhrase(item: KeyItem) {
    const { storedPhraseName, storedPhraseSecret } = storedPassphraseForm;
    if (storedPhraseName === "" || storedPhraseSecret === "") return;
    const stored = await runKeyOperation(
      () => storedPhrases.storeAndAssign(item, storedPhraseName, storedPhraseSecret),
      "keys.storePassphraseFailed",
    );
    if (!stored) {
      storedPassphraseForm.setStoredPhraseSecret("");
      return;
    }
    storedPassphraseForm.close();
  }

  async function unassignPhrase(item: KeyItem) {
    await runKeyOperation(() => storedPhrases.unassign(item), "keys.unassignPassphraseFailed");
  }

  return { changePassphrase, registerWithAgent, removeFromAgent, assignPhrase, storeAndAssignPhrase, unassignPhrase };
}

import { localApiBase } from "./remoteEngines";
import { apiClient } from "./client";
import { asRecord, postEmpty, postJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type ChangeMasterPasswordResult = components["schemas"]["ChangeMasterPasswordResult"];
export type PasswordVaultStatus = components["schemas"]["PasswordVaultStatus"];
export type PasswordEligibility = components["schemas"]["PasswordEligibility"];

export type VaultApi = {
  passwordVault(): Promise<PasswordVaultStatus>;
  initialiseVault(passphrase: string): Promise<PasswordVaultStatus>;
  unlockVault(passphrase: string): Promise<PasswordVaultStatus>;
  recoverCompatibleVault(passphrase: string): Promise<PasswordVaultStatus>;
  resetUnsupportedVault(passphrase: string): Promise<PasswordVaultStatus>;
  lockVault(): Promise<PasswordVaultStatus>;
  changeMasterPassword(
    current: string,
    next: string,
  ): Promise<ChangeMasterPasswordResult>;
  passwordEligibility(alias: string): Promise<PasswordEligibility>;
};

function validateVaultStatus(value: unknown): PasswordVaultStatus {
  return validateOpenAPISchema<PasswordVaultStatus>("PasswordVaultStatus", value);
}

function validatePasswordEligibility(value: unknown): PasswordEligibility {
  return validateOpenAPISchema<PasswordEligibility>("PasswordEligibility", value);
}

// The password vault itself: its lock state, the master password, and whether
// a host may be given a password. Stored values go through the credentials API.
// createVaultApi is this API of the engine at base (engineApiBase): this machine's, or a run target's as this
// machine's engine forwards it.
export function createVaultApi(base: string): VaultApi {
  return {
    async passwordVault() {
      return validateVaultStatus(await apiClient.read(`${base}/passwords`));
    },
    async initialiseVault(passphrase) {
      return validateVaultStatus(
        await postJSON<unknown>(`${base}/passwords/initialise`, { passphrase }),
      );
    },
    async unlockVault(passphrase) {
      return validateVaultStatus(
        await postJSON<unknown>(`${base}/passwords/unlock`, { passphrase }),
      );
    },
    async recoverCompatibleVault(passphrase) {
      return validateVaultStatus(
        await postJSON<unknown>(`${base}/passwords/recover-compatible-backup`, {
          passphrase,
        }),
      );
    },
    async resetUnsupportedVault(passphrase) {
      return validateVaultStatus(
        await postJSON<unknown>(`${base}/passwords/reset-unsupported`, {
          passphrase,
          acknowledged: true,
        }),
      );
    },
    async lockVault() {
      return validateVaultStatus(
        await postEmpty<unknown>(`${base}/passwords/lock`),
      );
    },
    async changeMasterPassword(current, next) {
      const answer = await postJSON<unknown>(`${base}/passwords/change`, {
        current,
        next,
      });
      const record = asRecord(answer);
      return { vault: validateVaultStatus(record.vault) };
    },
    async passwordEligibility(alias) {
      return validatePasswordEligibility(
        await apiClient.read(
          `${base}/passwords/${encodeURIComponent(alias)}/eligibility`,
        ),
      );
    },
  };
}

export const vaultApi = createVaultApi(localApiBase);

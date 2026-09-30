import type { PasswordVaultStatus } from "../api/vault";

// The shortest master password the engine accepts (secret.MinPassphraseLength).
const engineMinPassphraseLength = 4;

// vaultStatus is a PasswordVaultStatus with every field the engine always
// sends: an existing, unlocked vault with a master password and nothing saved
// in it. A test names only the fields it is about, so a field added to the
// status later is filled here once rather than in every test.
export function vaultStatus(overrides: Partial<PasswordVaultStatus> = {}): PasswordVaultStatus {
  return {
    exists: true,
    unlocked: true,
    passwordless: false,
    aliases: [],
    dedicatedKeyPassphrases: [],
    minPassphraseLength: engineMinPassphraseLength,
    ...overrides,
  };
}

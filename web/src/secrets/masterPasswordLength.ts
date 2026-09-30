// Below this length the engine still accepts a master password, but the lock
// screen and Settings suggest a longer one. It is the minimum the engine keeps
// for the sync key (internal/envelope.MinPassphraseLength), whose sealed data
// leaves this machine.
export const RECOMMENDED_MASTER_PASSWORD_LENGTH = 12;

// The engine counts a passphrase in Unicode code points
// (internal/envelope.DeriveWithMinimum). String.length would count an emoji or
// another character outside the BMP as two.
export function masterPasswordLength(masterPassword: string): number {
  return [...masterPassword].length;
}

// `minimum` is PasswordVaultStatus.minPassphraseLength. The engine owns the
// rule, so while its value is unknown no password passes.
export function meetsMasterPasswordMinimum(masterPassword: string, minimum: number | undefined): boolean {
  return minimum !== undefined && masterPasswordLength(masterPassword) >= minimum;
}

export function isAcceptedButShort(masterPassword: string, minimum: number | undefined): boolean {
  return meetsMasterPasswordMinimum(masterPassword, minimum) &&
    masterPasswordLength(masterPassword) < RECOMMENDED_MASTER_PASSWORD_LENGTH;
}

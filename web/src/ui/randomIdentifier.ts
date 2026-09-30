// The random values the web UI makes for itself. Both come from
// crypto.getRandomValues, which every browser offers, where crypto.randomUUID
// exists only in a secure context and would need a weaker fallback.

// 16 bytes (128 bits): nothing can guess a token, and identifiers made on
// different machines, such as synced shortcut presets, do not collide.
const randomByteCount = 16;

function randomHex(): string {
  const bytes = new Uint8Array(randomByteCount);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

// unguessableToken is for a value that must not be guessed by anything outside
// this page, such as the drag token another tab could try to forge.
export function unguessableToken(): string {
  return randomHex();
}

// newIdentifier is for a name that only has to differ from the others beside
// it: a pane, a tab, a transfer job, a shortcut preset. It is not a secret.
export function newIdentifier(): string {
  return randomHex();
}

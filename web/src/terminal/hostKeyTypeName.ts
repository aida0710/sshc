// hostKeyTypeName names a host key type as OpenSSH says it when it asks about a key ("ED25519 key fingerprint
// is …"), so the name on the screen matches the one in the terminal and in ssh-keygen's output.
export function hostKeyTypeName(keyType: string): string {
  if (keyType.startsWith("sk-ssh-ed25519")) return "ED25519-SK";
  if (keyType.startsWith("sk-ecdsa-")) return "ECDSA-SK";
  if (keyType.startsWith("ecdsa-")) return "ECDSA";
  if (keyType.startsWith("ssh-")) return keyType.slice("ssh-".length).toUpperCase();
  return keyType.toUpperCase();
}

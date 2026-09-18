import type { RemoteDragPayload } from "./transfers";

// The HTML drag data store is not isolated by origin: a page in another tab
// can start a drag whose data claims to be sshc rows, and the user can drop it
// on a pane here. So the rows themselves never travel in the data transfer.
// beginDrag keeps them in this registry and writes only an opaque token; a
// drop turns the token back into rows, and a token this page never issued
// resolves to nothing. Drags between two sshc windows therefore do not
// transfer rows — each window has its own registry.

const active = new Map<string, RemoteDragPayload>();

function mintToken(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

export function registerDrag(payload: RemoteDragPayload): string {
  const token = mintToken();
  active.set(token, payload);
  return token;
}

export function releaseDrag(token: string): void {
  active.delete(token);
}

export function payloadFor(token: string): RemoteDragPayload | null {
  return active.get(token) ?? null;
}

import { apiClient } from "./client";
import { issueAction, jsonHeaders } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type Credential = components["schemas"]["Credential"];
export type CredentialList = components["schemas"]["CredentialList"];
export type RevealCredentialResponse = components["schemas"]["RevealCredentialResponse"];
export type TOTPCodeSet = components["schemas"]["TOTPCodeSet"];

export type CredentialKind = "password" | "key_passphrase" | "totp";

export const CREDENTIAL_REVEAL_ACTION_KIND = "credential.reveal";

function credentialPath(kind: CredentialKind, name: string): string {
  return `/api/v1/credentials/${kind}/${encodeURIComponent(name)}`;
}

export type CredentialsApi = {
  credentials(): Promise<CredentialList>;
  storeCredential(
    kind: CredentialKind,
    name: string,
    secret: string,
  ): Promise<CredentialList>;
  revealCredential(
    kind: CredentialKind,
    name: string,
  ): Promise<RevealCredentialResponse>;
  totpCodes(name: string): Promise<TOTPCodeSet>;
  updateCredential(
    kind: CredentialKind,
    currentName: string,
    name: string,
    secret: string,
  ): Promise<CredentialList>;
  deleteCredential(kind: CredentialKind, name: string): Promise<CredentialList>;
  assignCredential(
    kind: CredentialKind,
    subject: string,
    name: string,
  ): Promise<CredentialList>;
  unassignCredential(
    kind: CredentialKind,
    subject: string,
  ): Promise<CredentialList>;
};

function validateCredentialList(value: unknown): CredentialList {
  return validateOpenAPISchema<CredentialList>("CredentialList", value);
}

function validateRevealCredential(value: unknown): RevealCredentialResponse {
  return validateOpenAPISchema<RevealCredentialResponse>("RevealCredentialResponse", value);
}

function validateTOTPCodeSet(value: unknown): TOTPCodeSet {
  return validateOpenAPISchema<TOTPCodeSet>("TOTPCodeSet", value);
}

// Named credentials (passwords, key passphrases, TOTP secrets) and their
// assignment to hosts and keys.
export const credentialsApi: CredentialsApi = {
  async credentials() {
    return validateCredentialList(await apiClient.read("/api/v1/credentials"));
  },
  async storeCredential(kind, name, secret) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, name), {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ secret }),
      }),
    );
  },
  async revealCredential(kind, name) {
    const token = await issueAction(
      CREDENTIAL_REVEAL_ACTION_KIND,
      `${kind}\n${name}`,
    );
    return validateRevealCredential(
      await apiClient.mutate<unknown>(`${credentialPath(kind, name)}/reveal`, {
        method: "POST",
        headers: { "X-SSHC-Action": token },
      }),
    );
  },
  async totpCodes(name) {
    const token = await issueAction(
      CREDENTIAL_REVEAL_ACTION_KIND,
      `totp\n${name}`,
    );
    return validateTOTPCodeSet(
      await apiClient.mutate<unknown>(
        `${credentialPath("totp", name)}/codes`,
        { method: "POST", headers: { "X-SSHC-Action": token } },
      ),
    );
  },
  async updateCredential(kind, currentName, name, secret) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, currentName), {
        method: "PATCH",
        headers: jsonHeaders,
        body: JSON.stringify({ name, secret }),
      }),
    );
  },
  async deleteCredential(kind, name) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(credentialPath(kind, name), {
        method: "DELETE",
      }),
    );
  },
  async assignCredential(kind, subject, name) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(`/api/v1/credentials/${kind}/assign`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ subject, name }),
      }),
    );
  },
  async unassignCredential(kind, subject) {
    return validateCredentialList(
      await apiClient.mutate<unknown>(
        `/api/v1/credentials/${kind}/assign/${encodeURIComponent(subject)}`,
        { method: "DELETE" },
      ),
    );
  },
};

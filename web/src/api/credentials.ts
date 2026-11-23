import { localApiBase } from "./remoteEngines";
import { apiClient } from "./client";
import { issueAction, patchJSON, putJSON } from "./guards";
import type { components } from "./schema";
import { validateOpenAPISchema } from "./validators.generated";

export type Credential = components["schemas"]["Credential"];
export type CredentialList = components["schemas"]["CredentialList"];
export type RevealCredentialResponse = components["schemas"]["RevealCredentialResponse"];
export type TOTPCodeSet = components["schemas"]["TOTPCodeSet"];

export type CredentialKind = "password" | "key_passphrase" | "totp";

const CREDENTIAL_REVEAL_ACTION_KIND = "credential.reveal";

function credentialPath(base: string, kind: CredentialKind, name: string): string {
  return `${base}/credentials/${kind}/${encodeURIComponent(name)}`;
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
// createCredentialsApi is this API of the engine at base (engineApiBase): this machine's, or a run target's as this
// machine's engine forwards it.
export function createCredentialsApi(base: string): CredentialsApi {
  return {
    async credentials() {
      return validateCredentialList(await apiClient.read(`${base}/credentials`));
    },
    async storeCredential(kind, name, secret) {
      return validateCredentialList(
        await putJSON<unknown>(credentialPath(base, kind, name), { secret }),
      );
    },
    async revealCredential(kind, name) {
      const token = await issueAction(
        CREDENTIAL_REVEAL_ACTION_KIND,
        `${kind}\n${name}`, base);
      return validateRevealCredential(
        await apiClient.mutate<unknown>(`${credentialPath(base, kind, name)}/reveal`, {
          method: "POST",
          headers: { "X-SSHC-Action": token },
        }),
      );
    },
    async totpCodes(name) {
      const token = await issueAction(
        CREDENTIAL_REVEAL_ACTION_KIND,
        `totp\n${name}`, base);
      return validateTOTPCodeSet(
        await apiClient.mutate<unknown>(
          `${credentialPath(base, "totp", name)}/codes`,
          { method: "POST", headers: { "X-SSHC-Action": token } },
        ),
      );
    },
    async updateCredential(kind, currentName, name, secret) {
      return validateCredentialList(
        await patchJSON<unknown>(credentialPath(base, kind, currentName), { name, secret }),
      );
    },
    async deleteCredential(kind, name) {
      return validateCredentialList(
        await apiClient.mutate<unknown>(credentialPath(base, kind, name), {
          method: "DELETE",
        }),
      );
    },
    async assignCredential(kind, subject, name) {
      return validateCredentialList(
        await putJSON<unknown>(`${base}/credentials/${kind}/assign`, { subject, name }),
      );
    },
    async unassignCredential(kind, subject) {
      return validateCredentialList(
        await apiClient.mutate<unknown>(
          `${base}/credentials/${kind}/assign/${encodeURIComponent(subject)}`,
          { method: "DELETE" },
        ),
      );
    },
  };
}

export const credentialsApi = createCredentialsApi(localApiBase);

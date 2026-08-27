import { describe, expect, it, vi } from "vitest";
import type { HostDetail } from "../api/config";
import type {
  Credential,
  IntegrationsApi,
  PasswordEligibility,
  PasswordVaultStatus,
} from "../api/integrations";
import type { KeyInventoryResponse, KeyItem, KeysApi } from "../keys/api";
import {
  loadConnectionSavedState,
  summarizeConnection,
  type ConnectionSavedState,
} from "./connectionSavedState";

const privateKey: KeyItem = {
  id: "0123456789abcdef0123456789abcdef",
  relativePath: "id_work",
  kind: "private_key",
  container: "OPENSSH PRIVATE KEY",
  algorithm: "ed25519",
  keyType: "ssh-ed25519",
  bits: 256,
  encrypted: true,
  fingerprint: "SHA256:work",
  comment: "work",
  permission: "0600",
  permissionRisk: false,
  sizeBytes: 444,
  references: [],
  notes: [],
};

const inventory: KeyInventoryResponse = {
  items: [privateKey],
  unreadable: [],
  agentDelegations: [],
  unresolvedReferences: [],
  agentAvailable: false,
  agentIdentities: [],
};

const unlockedVault: PasswordVaultStatus = {
  exists: true,
  unlocked: true,
  aliases: ["edge"],
  dedicatedKeyPassphrases: [],
  minPassphraseLength: 12,
};

const eligibility: PasswordEligibility = {
  alias: "edge",
  storable: true,
  blockers: [],
  warnings: [],
  hostName: "edge.example",
  port: "2200",
};

const credentials: Credential[] = [
  { kind: "password", name: "office", uses: ["edge"], hosts: ["edge"] },
  { kind: "key_passphrase", name: "team-key", uses: ["id_work"], hosts: ["edge"] },
];

function detailWithIdentityFile(): HostDetail {
  return {
    form: {
      entry: {
        identity: { path: "connections/work/edge.conf", alias: "edge" },
        file: {
          path: "connections/work/edge.conf",
          absolute: "/home/tester/.ssh/connections/work/edge.conf",
        },
        line: 1,
        patterns: ["edge"],
        editable: true,
        group: "work",
      },
      fields: [
        { line: 2, keyword: "HostName", values: ["edge.example"], category: "basic", editable: true },
        { line: 3, keyword: "User", values: ["deploy"], category: "basic", editable: true },
        { line: 4, keyword: "Port", values: ["2200"], category: "basic", editable: true },
        { line: 5, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
      ],
      raw: "Host edge\n\tHostName edge.example\n",
      comment: "",
      commentLines: 0,
    },
    metadata: {
      identity: { path: "connections/work/edge.conf", alias: "edge" },
    },
    effective: {
      alias: "edge",

      entries: [],
    },
    file: {
      file: {
        path: "connections/work/edge.conf",
        absolute: "/home/tester/.ssh/connections/work/edge.conf",
      },
      contents: "Host edge\n\tHostName edge.example\n",
      digest: "digest",
      editable: true,
      exists: true,
    },
  };
}

function apis(overrides: {
  inventory?: KeysApi["inventory"];
  passwordVault?: IntegrationsApi["passwordVault"];
  credentials?: IntegrationsApi["credentials"];
  passwordEligibility?: IntegrationsApi["passwordEligibility"];
} = {}) {
  return {
    keys: {
      inventory: overrides.inventory ?? vi.fn().mockResolvedValue(inventory),
    } as Pick<KeysApi, "inventory">,
    secrets: {
      passwordVault: overrides.passwordVault ?? vi.fn().mockResolvedValue(unlockedVault),
      credentials: overrides.credentials ?? vi.fn().mockResolvedValue({
        credentials,
        dedicatedKeyPassphrases: [],
        keyHostUsageComplete: true,
      }),
      passwordEligibility: overrides.passwordEligibility ?? vi.fn().mockResolvedValue(eligibility),
    } as Pick<IntegrationsApi, "passwordVault" | "credentials" | "passwordEligibility">,
  };
}

describe("connection saved state", () => {
  it("treats a direct IdentityFile none as agent or inherited authentication", () => {
    const detail = detailWithIdentityFile();
    detail.form.fields = detail.form.fields.map((field) =>
      field.keyword === "IdentityFile" ? { ...field, values: ["none"] } : field,
    );
    const saved: ConnectionSavedState = {
      detail,
      keys: { status: "ready", value: [privateKey] },
      vault: { status: "ready", value: { ...unlockedVault, aliases: [] } },
      credentials: { status: "ready", value: [] },
      eligibility: { status: "ready", value: eligibility },
    };

    expect(summarizeConnection(saved).privateKey).toEqual({ state: "none" });
  });

  it("keeps successful vault resources when key inventory fails", async () => {
    const dependencies = apis({ inventory: vi.fn().mockRejectedValue(new Error("unreadable")) });

    const saved = await loadConnectionSavedState(detailWithIdentityFile(), dependencies.keys, dependencies.secrets);

    expect(saved.keys).toEqual({ status: "failed" });
    expect(saved.vault).toEqual({ status: "ready", value: unlockedVault });
    expect(saved.credentials).toEqual({ status: "ready", value: credentials });
    expect(saved.eligibility).toEqual({ status: "ready", value: eligibility });
  });

  it("reports credential failure as unavailable instead of no saved password", async () => {
    const dependencies = apis({ credentials: vi.fn().mockRejectedValue(new Error("broken")) });
    const saved = await loadConnectionSavedState(detailWithIdentityFile(), dependencies.keys, dependencies.secrets);

    expect(saved.credentials).toEqual({ status: "failed" });
    expect(summarizeConnection(saved).accountPassword).toEqual({ state: "unavailable" });
  });

  it("distinguishes a locked credential resource from a failed or empty one", async () => {
    const locked = { ...unlockedVault, unlocked: false };
    const credentialCall = vi.fn();
    const dependencies = apis({
      passwordVault: vi.fn().mockResolvedValue(locked),
      credentials: credentialCall,
    });

    const saved = await loadConnectionSavedState(detailWithIdentityFile(), dependencies.keys, dependencies.secrets);

    expect(saved.credentials).toEqual({ status: "locked" });
    expect(credentialCall).not.toHaveBeenCalled();
    expect(summarizeConnection(saved).accountPassword).toEqual({ state: "locked" });
  });

  it("projects the committed endpoint and independent named authentication assignments", () => {
    const saved: ConnectionSavedState = {
      detail: detailWithIdentityFile(),
      keys: { status: "ready", value: [privateKey] },
      vault: { status: "ready", value: unlockedVault },
      credentials: { status: "ready", value: credentials },
      eligibility: { status: "ready", value: eligibility },
    };

    expect(summarizeConnection(saved)).toEqual({
      alias: "edge",
      endpoint: "deploy@edge.example:2200",
      group: "work",
      privateKey: {
        state: "known",
        path: "id_work",
        fingerprint: "SHA256:work",
        encrypted: true,
      },
      keyPassphrase: { state: "named", name: "team-key" },
      accountPassword: { state: "named", name: "office" },
    });
  });

  it("distinguishes dedicated, confirmed-empty, and unencrypted key states", () => {
    const dedicated: ConnectionSavedState = {
      detail: detailWithIdentityFile(),
      keys: { status: "ready", value: [privateKey] },
      vault: {
        status: "ready",
        value: { ...unlockedVault, aliases: ["edge"], dedicatedKeyPassphrases: ["id_work"] },
      },
      credentials: { status: "ready", value: [] },
      eligibility: { status: "ready", value: eligibility },
    };
    expect(summarizeConnection(dedicated).accountPassword).toEqual({ state: "dedicated" });
    expect(summarizeConnection(dedicated).keyPassphrase).toEqual({ state: "dedicated" });

    const empty: ConnectionSavedState = {
      ...dedicated,
      vault: { status: "ready", value: { ...unlockedVault, aliases: [], dedicatedKeyPassphrases: [] } },
    };
    expect(summarizeConnection(empty).accountPassword).toEqual({ state: "none" });
    expect(summarizeConnection(empty).keyPassphrase).toEqual({ state: "none" });

    const plain: ConnectionSavedState = {
      ...empty,
      keys: { status: "ready", value: [{ ...privateKey, encrypted: false }] },
    };
    expect(summarizeConnection(plain).keyPassphrase).toEqual({ state: "not_needed" });
  });
});

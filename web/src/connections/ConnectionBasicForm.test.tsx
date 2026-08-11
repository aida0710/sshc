import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { HostDetail, UpdateConnectionRequest } from "../api/config";
import type { Problem } from "../api/client";
import type { IntegrationsApi } from "../api/integrations";
import type { KeyInventoryResponse, KeysApi } from "../keys/api";
import { ConnectionBasicForm } from "./ConnectionBasicForm";
import type { ConnectionSavedState } from "./connectionSavedState";

const privateKey = {
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

const secondKey = {
  ...privateKey,
  id: "fedcba9876543210fedcba9876543210",
  relativePath: "keys/home/id_home",
  fingerprint: "SHA256:home",
};

const unencryptedKey = {
  ...privateKey,
  id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  relativePath: "id_plain",
  encrypted: false,
  fingerprint: "SHA256:plain",
};

const inventory: KeyInventoryResponse = {
  items: [privateKey, secondKey, unencryptedKey],
  unreadable: [],
  agentDelegations: [],
  unresolvedReferences: [],
  agentAvailable: false,
  agentIdentities: [],
};

function buildDetail(fields: HostDetail["form"]["fields"] = []): HostDetail {
  return {
    form: {
      entry: {
        identity: { path: "connections/work/edge.conf", alias: "edge" },
        file: { path: "connections/work/edge.conf", absolute: "/home/tester/.ssh/connections/work/edge.conf" },
        line: 1,
        patterns: ["edge"],
        editable: true,
      },
      fields,
      raw: "Host edge\n",
      comment: "",
      commentLines: 0,
    },
    metadata: { identity: { path: "connections/work/edge.conf", alias: "edge" }, favourite: false },
    effective: {
      alias: "edge",
      approximate: true,
      entries: [
        { keyword: "HostName", values: ["inherited.example"], source: { path: "config", line: 8 } },
        { keyword: "User", values: ["deploy"], source: { path: "connections/work/defaults.conf", line: 3 } },
      ],
    },
    file: {
      file: { path: "connections/work/edge.conf", absolute: "/home/tester/.ssh/connections/work/edge.conf" },
      contents: "Host edge\n",
      digest: "digest",
      editable: true,
      exists: true,
    },
  };
}

type HarnessOverrides = {
  detail?: HostDetail;
  onSave?: (request: UpdateConnectionRequest) => Promise<void>;
  passwordVault?: IntegrationsApi["passwordVault"];
  credentials?: IntegrationsApi["credentials"];
  passwordEligibility?: IntegrationsApi["passwordEligibility"];
  initialiseVault?: IntegrationsApi["initialiseVault"];
  unlockVault?: IntegrationsApi["unlockVault"];
  inventory?: KeysApi["inventory"];
  problem?: Problem | null;
  preferredKey?: { privateKeyId: string; privateRelativePath: string } | null;
  onPreferredKeyApplied?: () => void;
  savedState?: ConnectionSavedState;
  onDirtyChange?: (dirty: boolean) => void;
  onDiscardReady?: (discard: (() => void) | null) => void;
};

function renderForm(overrides: HarnessOverrides = {}) {
  const onSave = overrides.onSave ?? vi.fn().mockResolvedValue(undefined);
  const passwordVault = overrides.passwordVault ?? vi.fn().mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  });
  const credentials = overrides.credentials ?? vi.fn().mockResolvedValue({
    credentials: [
      { kind: "password", name: "office", uses: ["bastion"] },
      { kind: "key_passphrase", name: "id_work", uses: ["id_work"] },
    ],
  });
  const passwordEligibility = overrides.passwordEligibility ?? vi.fn().mockResolvedValue({
    alias: "edge", storable: true, blockers: [], warnings: [],
  });
  const initialiseVault = overrides.initialiseVault ?? vi.fn().mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  });
  const unlockVault = overrides.unlockVault ?? vi.fn().mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  });
  const keyInventory = overrides.inventory ?? vi.fn().mockResolvedValue(inventory);

  const rendered = render(
    <ConnectionBasicForm
      detail={overrides.detail ?? buildDetail()}
      problem={overrides.problem ?? null}
      onSave={onSave}
      keys={{ inventory: keyInventory } as Pick<KeysApi, "inventory">}
      secrets={{
        passwordVault, credentials, passwordEligibility, initialiseVault, unlockVault,
      } as Pick<
        IntegrationsApi,
        "passwordVault" | "credentials" | "passwordEligibility" | "initialiseVault" | "unlockVault"
      >}
      preferredKey={overrides.preferredKey}
      onPreferredKeyApplied={overrides.onPreferredKeyApplied}
      savedState={overrides.savedState}
      onDirtyChange={overrides.onDirtyChange}
      onDiscardReady={overrides.onDiscardReady}
    />,
  );
  return {
    ...rendered,
    onSave,
    passwordVault,
    credentials,
    passwordEligibility,
    initialiseVault,
    unlockVault,
    keyInventory,
  };
}

afterEach(() => vi.restoreAllMocks());

describe("ConnectionBasicForm", () => {

  it("stages a freshly generated key from a fresh inventory and applies it only on Save", async () => {
    const user = userEvent.setup();
    const onPreferredKeyApplied = vi.fn();
    const harness = renderForm({
      preferredKey: { privateKeyId: secondKey.id, privateRelativePath: secondKey.relativePath },
      onPreferredKeyApplied,
    });

    await waitFor(() => expect(screen.getByLabelText("SSH private key")).toHaveValue(secondKey.id));
    expect(screen.getByText(/staged for this connection/)).toBeInTheDocument();
    expect(harness.onSave).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith(expect.objectContaining({
      identityFile: { action: "set", keyId: secondKey.id },
    }));
    expect(onPreferredKeyApplied).toHaveBeenCalledOnce();
  });

  it("consumes a generated-key handoff when another key is saved instead", async () => {
    const user = userEvent.setup();
    const onPreferredKeyApplied = vi.fn();
    const harness = renderForm({
      preferredKey: { privateKeyId: secondKey.id, privateRelativePath: secondKey.relativePath },
      onPreferredKeyApplied,
    });

    await waitFor(() => expect(screen.getByLabelText("SSH private key")).toHaveValue(secondKey.id));
    await user.selectOptions(screen.getByLabelText("SSH private key"), privateKey.id);
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith(expect.objectContaining({
      identityFile: { action: "set", keyId: privateKey.id },
    }));
    expect(onPreferredKeyApplied).toHaveBeenCalledOnce();
  });

  it("places stable server validation failures beside the affected control", async () => {
    const first = renderForm({
      problem: { code: "connection_hostname_invalid", message: "invalid host" },
    });
    expect(screen.getByText("Enter a DNS name, IPv4 address, or unbracketed IPv6 address.")).toBeInTheDocument();
    first.unmount();

    const second = renderForm({
      problem: { code: "identity_file_invalid", message: "invalid key" },
    });
    expect(screen.getByText("That SSH private key is no longer selectable. Reload and choose it again.")).toBeInTheDocument();
    second.unmount();

    renderForm({
      problem: { code: "credential_already_exists", message: "credential exists" },
    });
    expect(await screen.findByText(/saved password name already exists/i)).toBeInTheDocument();
  });
  it("always renders sparse connection fields without materialising inherited defaults", async () => {
    renderForm();

    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("inherited.example");
    expect(screen.getByLabelText("User")).toHaveValue("deploy");
    expect(screen.getByLabelText("Port")).toHaveValue(22);
    expect(screen.getByText(/Inherited from config:8/)).toBeInTheDocument();
    expect(screen.getByText(/SSH default/)).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();
  });

  it("emits semantic set and inherit changes in one request", async () => {
    const user = userEvent.setup();
    const detail = buildDetail([
      { line: 2, keyword: "User", values: ["root"], category: "basic", editable: true },
      { line: 3, keyword: "Port", values: ["2200"], category: "basic", editable: true },
    ]);
    const harness = renderForm({ detail });
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    await user.clear(screen.getByLabelText("Host name or IP address"));
    await user.type(screen.getByLabelText("Host name or IP address"), "198.51.100.7");
    await user.clear(screen.getByLabelText("User"));
    await user.click(screen.getByRole("button", { name: "Use inherited/default port" }));
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith({
      identity: detail.form.entry.identity,
      base: detail.file.contents,
      hostName: { action: "set", value: "198.51.100.7" },
      user: { action: "inherit" },
      port: { action: "inherit" },
      password: { kind: "unchanged" },
      keyPassphrase: { kind: "unchanged" },
    });
  });

  it("keeps key and password changes independent and never prefills a password", async () => {
    const user = userEvent.setup();
    const detail = buildDetail([
      { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
    ]);
    const harness = renderForm({ detail });
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    expect(screen.getByLabelText("SSH private key")).toHaveValue(privateKey.id);
    await user.selectOptions(screen.getByLabelText("SSH private key"), secondKey.id);
    await user.selectOptions(screen.getByLabelText("Stored password action"), "dedicated_password");
    expect(screen.getByLabelText("Connection password")).toHaveValue("");
    await user.type(screen.getByLabelText("Connection password"), "new-secret");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith({
      identity: detail.form.entry.identity,
      base: detail.file.contents,
      identityFile: { action: "set", keyId: secondKey.id },
      password: { kind: "dedicated_password", password: "new-secret" },
      keyPassphrase: { kind: "unchanged" },
    });
  });

  it("requires unlock before any save while preserving non-secret drafts", async () => {
    const user = userEvent.setup();
    const harness = renderForm({
      passwordVault: vi.fn().mockResolvedValue({
        exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [],
      }),
    });

    await user.clear(screen.getByLabelText("Host name or IP address"));
    await user.type(screen.getByLabelText("Host name or IP address"), "draft.example");
    expect(await screen.findByText("Unlock the encrypted vault before saving Basic settings.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();

    await user.type(screen.getByLabelText("Master password"), "the master password");
    await user.click(screen.getByRole("button", { name: "Unlock vault" }));
    await waitFor(() => expect(harness.unlockVault).toHaveBeenCalledWith("the master password"));
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("draft.example");
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeEnabled();
  });

  it("saves config-only changes from shared state when credential metadata is unavailable", async () => {
    const user = userEvent.setup();
    const detail = buildDetail([
      { line: 2, keyword: "HostName", values: ["edge.example"], category: "basic", editable: true },
      { line: 3, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
    ]);
    const savedState: ConnectionSavedState = {
      detail,
      keys: { status: "ready", value: [privateKey, secondKey, unencryptedKey] },
      vault: {
        status: "ready",
        value: {
          exists: true,
          unlocked: true,
          aliases: ["edge"],
          dedicatedKeyPassphrases: [],
          minPassphraseLength: 12,
        },
      },
      credentials: { status: "failed" },
      eligibility: {
        status: "ready",
        value: { alias: "edge", storable: true, blockers: [], warnings: [] },
      },
    };
    const onDirtyChange = vi.fn();
    const harness = renderForm({
      detail,
      savedState,
      onDirtyChange,
      inventory: vi.fn().mockRejectedValue(new Error("must not load")),
      passwordVault: vi.fn().mockRejectedValue(new Error("must not load")),
      credentials: vi.fn().mockRejectedValue(new Error("must not load")),
      passwordEligibility: vi.fn().mockRejectedValue(new Error("must not load")),
    });

    await user.clear(screen.getByLabelText("Host name or IP address"));
    await user.type(screen.getByLabelText("Host name or IP address"), "retry.example");
    expect(screen.getByText("Saved password options could not be loaded.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Stored password action")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith({
      identity: detail.form.entry.identity,
      base: detail.file.contents,
      hostName: { action: "set", value: "retry.example" },
      password: { kind: "unchanged" },
      keyPassphrase: { kind: "unchanged" },
    });
    expect(onDirtyChange).toHaveBeenCalledWith(true);
    expect(harness.keyInventory).not.toHaveBeenCalled();
    expect(harness.passwordVault).not.toHaveBeenCalled();
    expect(harness.credentials).not.toHaveBeenCalled();
  });

  it("registers a discard action that restores committed fields and clears secrets", async () => {
    const user = userEvent.setup();
    let discard: (() => void) | null = null;
    renderForm({
      onDiscardReady: (next) => {
        discard = next;
      },
    });
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    await user.clear(screen.getByLabelText("Host name or IP address"));
    await user.type(screen.getByLabelText("Host name or IP address"), "draft.example");
    await user.selectOptions(screen.getByLabelText("Stored password action"), "dedicated_password");
    await user.type(screen.getByLabelText("Connection password"), "must disappear");
    expect(discard).not.toBeNull();
    act(() => discard!());

    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("inherited.example");
    expect(screen.queryByLabelText("Connection password")).not.toBeInTheDocument();
  });

  it("offers an explicit discard action in the save bar", async () => {
    const user = userEvent.setup();
    renderForm();
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    await user.clear(screen.getByLabelText("Host name or IP address"));
    await user.type(screen.getByLabelText("Host name or IP address"), "draft.example");
    await user.click(screen.getByRole("button", { name: "Discard changes" }));

    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("inherited.example");
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();
  });

  it("requires explicit confirmation before removing an assigned password", async () => {
    const user = userEvent.setup();
    const detail = buildDetail();
    const harness = renderForm({
      detail,
      passwordVault: vi.fn().mockResolvedValue({
        exists: true, unlocked: true, aliases: ["edge"], dedicatedKeyPassphrases: [],
      }),
      credentials: vi.fn().mockResolvedValue({
        credentials: [{ kind: "password", name: "office", uses: ["edge", "bastion"] }],
      }),
    });
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    expect(screen.getByText("Assigned: office")).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Stored password action"), "remove");
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: "Confirm stored password removal" }));
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith({
      identity: detail.form.entry.identity,
      base: detail.file.contents,
      password: { kind: "remove" },
      keyPassphrase: { kind: "unchanged" },
    });
  });

  it("leaves duplicate and custom direct authentication fields read-only for Advanced", async () => {
    renderForm({
      detail: buildDetail([
        { line: 2, keyword: "HostName", values: ["one.example"], category: "basic", editable: true },
        { line: 3, keyword: "hostname", values: ["two.example"], category: "basic", editable: true, duplicate: true },
        { line: 4, keyword: "IdentityFile", values: ["/opt/custom/key"], category: "basic", editable: true },
      ]),
    });

    expect(screen.getByLabelText("Host name or IP address")).toBeDisabled();
    expect(screen.getByText(/multiple direct HostName values/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());
    expect(screen.getByLabelText("SSH private key")).toBeDisabled();
    expect(screen.getByText(/custom IdentityFile path/i)).toBeInTheDocument();
  });

  it("clears a rejected password but preserves the connection draft", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockRejectedValue(new Error("conflict"));
    renderForm({ onSave });
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    await user.clear(screen.getByLabelText("Host name or IP address"));
    await user.type(screen.getByLabelText("Host name or IP address"), "retry.example");
    await user.selectOptions(screen.getByLabelText("Stored password action"), "dedicated_password");
    await user.type(screen.getByLabelText("Connection password"), "must-clear");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Basic settings could not be saved");
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("retry.example");
    expect(screen.getByLabelText("Connection password")).toHaveValue("");
  });

  it("refreshes a password-only success even when the SSH file bytes do not change", async () => {
    const user = userEvent.setup();
    const passwordVault = vi.fn()
      .mockResolvedValueOnce({
        exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
      })
      .mockResolvedValueOnce({
        exists: true, unlocked: true, aliases: ["edge"], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
      });
    renderForm({ passwordVault });
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());

    await user.selectOptions(screen.getByLabelText("Stored password action"), "dedicated_password");
    await user.type(screen.getByLabelText("Connection password"), "password-only");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(await screen.findByText(/connection-only password is assigned/i)).toBeInTheDocument();
    expect(screen.getByLabelText("Stored password action")).toHaveValue("unchanged");
    expect(screen.queryByLabelText("Connection password")).not.toBeInTheDocument();
    expect(passwordVault).toHaveBeenCalledTimes(2);
  });

  it("shows no key-passphrase editor for no key, custom path, or multiple direct keys", async () => {
    const first = renderForm();
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());
    expect(screen.queryByLabelText("New saved key passphrase")).not.toBeInTheDocument();
    first.unmount();

    const custom = renderForm({
      detail: buildDetail([
        { line: 2, keyword: "IdentityFile", values: ["/opt/custom/key"], category: "basic", editable: true },
      ]),
    });
    await waitFor(() => expect(screen.getByLabelText("SSH private key")).toBeDisabled());
    expect(screen.queryByLabelText("New saved key passphrase")).not.toBeInTheDocument();
    custom.unmount();

    renderForm({
      detail: buildDetail([
        { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
        { line: 3, keyword: "IdentityFile", values: ["~/.ssh/id_plain"], category: "basic", editable: true },
      ]),
    });
    await waitFor(() => expect(screen.getByLabelText("SSH private key")).toBeDisabled());
    expect(screen.queryByLabelText("New saved key passphrase")).not.toBeInTheDocument();
  });

  it("explains that an unencrypted selected key needs no saved passphrase", async () => {
    renderForm({
      detail: buildDetail([
        { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_plain"], category: "basic", editable: true },
      ]),
    });
    expect(await screen.findByText("This private key is not encrypted, so it needs no saved passphrase.")).toBeInTheDocument();
    expect(screen.queryByLabelText("New saved key passphrase")).not.toBeInTheDocument();
  });

  it("distinguishes unsaved, shared named, and key-dedicated passphrases without prefilling values", async () => {
    const detail = buildDetail([
      { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
    ]);
    const unsaved = renderForm({
      detail,
      credentials: vi.fn().mockResolvedValue({ credentials: [] }),
    });
    expect(await screen.findByText("No passphrase is saved for this key.")).toBeInTheDocument();
    expect(screen.getByLabelText("New saved key passphrase")).toHaveValue("");
    unsaved.unmount();

    const shared = renderForm({
      detail,
      credentials: vi.fn().mockResolvedValue({
        credentials: [{ kind: "key_passphrase", name: "team-key", uses: ["id_work", "id_other"] }],
      }),
    });
    expect(await screen.findByText(/uses the shared saved passphrase “team-key”/i)).toBeInTheDocument();
    expect(screen.getByText(/also used by 1 other key/i)).toBeInTheDocument();
    shared.unmount();

    renderForm({
      detail,
      passwordVault: vi.fn().mockResolvedValue({
        exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: ["id_work"], minPassphraseLength: 12,
      }),
    });
    expect(await screen.findByText("A passphrase is saved only for this key.")).toBeInTheDocument();
  });

  it("requires matching key-passphrase fields and sends one mutation with the Basic save", async () => {
    const user = userEvent.setup();
    const detail = buildDetail([
      { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
    ]);
    const harness = renderForm({
      detail,
      credentials: vi.fn().mockResolvedValue({ credentials: [] }),
    });
    await screen.findByText("No passphrase is saved for this key.");
    await user.type(screen.getByLabelText("New saved key passphrase"), "correct phrase");
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();
    await user.type(screen.getByLabelText("Confirm saved key passphrase"), "wrong phrase");
    expect(screen.getByText("The key passphrases do not match.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();
    await user.clear(screen.getByLabelText("Confirm saved key passphrase"));
    await user.type(screen.getByLabelText("Confirm saved key passphrase"), "correct phrase");
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(harness.onSave).toHaveBeenCalledWith({
      identity: detail.form.entry.identity,
      base: detail.file.contents,
      password: { kind: "unchanged" },
      keyPassphrase: { kind: "set_dedicated", keyId: privateKey.id, passphrase: "correct phrase" },
    });
  });

  it("does not ignore a confirmation-only key-passphrase draft", async () => {
    const user = userEvent.setup();
    renderForm({
      detail: buildDetail([
        { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
      ]),
      credentials: vi.fn().mockResolvedValue({ credentials: [] }),
    });

    await user.type(await screen.findByLabelText("Confirm saved key passphrase"), "confirmation only");
    expect(screen.getByText("The key passphrases do not match.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save Basic settings" })).toBeDisabled();
  });

  it("keeps a key-passphrase draft when the independent account-password action changes", async () => {
    const user = userEvent.setup();
    renderForm({
      detail: buildDetail([
        { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
      ]),
      credentials: vi.fn().mockResolvedValue({ credentials: [] }),
    });

    await user.type(await screen.findByLabelText("New saved key passphrase"), "independent phrase");
    await user.type(screen.getByLabelText("Confirm saved key passphrase"), "independent phrase");
    await user.selectOptions(screen.getByLabelText("Stored password action"), "dedicated_password");
    expect(screen.getByLabelText("New saved key passphrase")).toHaveValue("independent phrase");
    expect(screen.getByLabelText("Confirm saved key passphrase")).toHaveValue("independent phrase");
  });

  it("clears both key-passphrase fields when the selected key changes", async () => {
    const user = userEvent.setup();
    renderForm({
      detail: buildDetail([
        { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
      ]),
    });
    await user.type(await screen.findByLabelText("New saved key passphrase"), "must clear");
    await user.type(screen.getByLabelText("Confirm saved key passphrase"), "must clear");
    await user.selectOptions(screen.getByLabelText("SSH private key"), secondKey.id);
    expect(screen.getByLabelText("New saved key passphrase")).toHaveValue("");
    expect(screen.getByLabelText("Confirm saved key passphrase")).toHaveValue("");
  });

  it("clears submitted key-passphrase fields after both success and failure", async () => {
    const user = userEvent.setup();
    const detail = buildDetail([
      { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
    ]);
    const success = renderForm({ detail });
    await user.type(await screen.findByLabelText("New saved key passphrase"), "success phrase");
    await user.type(screen.getByLabelText("Confirm saved key passphrase"), "success phrase");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));
    await waitFor(() => expect(screen.getByLabelText("New saved key passphrase")).toHaveValue(""));
    expect(screen.getByLabelText("Confirm saved key passphrase")).toHaveValue("");
    success.unmount();

    renderForm({ detail, onSave: vi.fn().mockRejectedValue(new Error("wrong")) });
    await user.type(await screen.findByLabelText("New saved key passphrase"), "failure phrase");
    await user.type(screen.getByLabelText("Confirm saved key passphrase"), "failure phrase");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));
    await waitFor(() => expect(screen.getByLabelText("New saved key passphrase")).toHaveValue(""));
    expect(screen.getByLabelText("Confirm saved key passphrase")).toHaveValue("");
  });

  it("shows wrong and stale key-passphrase problems beside the editor", async () => {
    const detail = buildDetail([
      { line: 2, keyword: "IdentityFile", values: ["~/.ssh/id_work"], category: "basic", editable: true },
    ]);
    const wrong = renderForm({ detail, problem: { code: "wrong_passphrase", message: "wrong" } });
    expect(await screen.findByText("That passphrase does not unlock the selected private key.")).toBeInTheDocument();
    wrong.unmount();
    renderForm({ detail, problem: { code: "external_change", message: "changed" } });
    expect(await screen.findByText("The selected private key changed. Reload before saving its passphrase.")).toBeInTheDocument();
  });
});

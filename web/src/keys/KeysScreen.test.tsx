import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KeysScreen } from "./KeysScreen";
import type { KeyInventoryResponse, KeysApi } from "./api";
import type { IntegrationsApi } from "../api/integrations";

function buildSecrets(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  const listed = {
    credentials: [
      { kind: "key_passphrase", name: "build-key", uses: [] },
      { kind: "password", name: "office-vm", uses: ["web-1"] },
    ],
  };
  return {
    passwordVault: vi.fn().mockResolvedValue({
      exists: true,
      unlocked: true,
      aliases: [],
      dedicatedKeyPassphrases: [],
      minPassphraseLength: 12,
    }),
    credentials: vi.fn().mockResolvedValue(listed),
    storeCredential: vi.fn().mockResolvedValue(listed),
    assignCredential: vi.fn().mockResolvedValue(listed),
    unassignCredential: vi.fn().mockResolvedValue(listed),
    ...overrides,
  } as unknown as IntegrationsApi;
}

afterEach(() => {
  vi.restoreAllMocks();
});

async function openKeyDetails(row: HTMLElement): Promise<HTMLElement> {
  const toggle = within(row).getByRole("button", { name: /(?:Show|Hide) details/ });
  if (toggle.getAttribute("aria-expanded") !== "true") await userEvent.click(toggle);
  return screen.getByRole("group", { name: "Key actions" });
}

async function openStoredPassphrase(row: HTMLElement) {
  const actions = await openKeyDetails(row);
  await userEvent.click(within(actions).getByRole("button", { name: "Save passphrase" }));
}

function buildInventory(): KeyInventoryResponse {
  return {
    items: [
      {
        id: "key-one",
        relativePath: "id_work",
        kind: "private_key",
        container: "OPENSSH PRIVATE KEY",
        algorithm: "ed25519",
        keyType: "ssh-ed25519",
        bits: 256,
        encrypted: true,
        fingerprint: "SHA256:abcdef",
        comment: "aida@laptop",
        permission: "0600",
        permissionRisk: false,
        sizeBytes: 444,
        references: [
          {
            directive: "IdentityFile",
            configPath: "/home/.ssh/config",
            line: 2,
            condition: "Host build-*",
            hostPatterns: ["build-*"],
            value: "~/.ssh/id_work",
          },
        ],
        notes: [],
      },
      {
        id: "key-two",
        relativePath: "legacy",
        kind: "private_key",
        container: "RSA PRIVATE KEY",
        algorithm: "rsa",
        keyType: "ssh-rsa",
        bits: 2048,
        encrypted: true,
        fingerprint: "",
        comment: "",
        permission: "0644",
        permissionRisk: true,
        sizeBytes: 1700,
        references: [],
        notes: ["fingerprint_unavailable"],
      },
    ],
    unreadable: [],
    agentDelegations: [],
    unresolvedReferences: [],
    agentAvailable: false,
    agentIdentities: [],
  };
}

function inventoryWithAgent(): KeyInventoryResponse {
  return { ...buildInventory(), agentAvailable: true };
}

function inventoryWithLoadedKey(): KeyInventoryResponse {
  return {
    ...buildInventory(),
    agentAvailable: true,
    agentIdentities: [
      { bits: 256, fingerprint: "SHA256:abcdef", comment: "aida@laptop", algorithm: "ED25519" },
    ],
  };
}

function buildApi(overrides: Partial<KeysApi> = {}): KeysApi {
  return {
    inventory: vi.fn().mockResolvedValue(buildInventory()),
    algorithms: vi.fn().mockResolvedValue({
      variants: [
        { algorithm: "ed25519", bits: 256, label: "Ed25519", inProcess: true, reason: "" },
        {
          algorithm: "ed25519-sk",
          bits: 0,
          label: "Ed25519 security key",
          inProcess: false,
          reason: "hardware_token_required",
        },
      ],
      source: "ssh -Q key",
    }),
    generate: vi.fn().mockResolvedValue({
      id: "key-new",
      relativePath: "id_new",
      publicRelativePath: "id_new.pub",
      fingerprint: "SHA256:new",
      keyType: "ssh-ed25519",
      bits: 256,
      encrypted: true,
      transactionId: "tx",
    }),
    hardwareCommand: vi.fn().mockResolvedValue({
      algorithm: "ed25519-sk",
      command: ["ssh-keygen", "-t", "ed25519-sk", "-f", "/home/.ssh/id_yubikey"],
      note: "run_in_terminal",
    }),
    changePassphrase: vi.fn(),
    reveal: vi.fn(),
    publicKey: vi.fn().mockResolvedValue({
      id: "key-three",
      relativePath: "id_work.pub",
      publicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmU aida@laptop\n",
      fingerprint: "SHA256:abcdef",
      comment: "aida@laptop",
    }),
    registerWithAgent: vi.fn().mockResolvedValue({
      id: "key-one",
      relativePath: "id_work",
      fingerprint: "SHA256:abcdef",
      lifetimeSeconds: 0,
      identities: [],
    }),
    deregisterFromAgent: vi.fn().mockResolvedValue({
      id: "key-one",
      agentAvailable: true,
      identities: [],
    }),
    relocate: vi.fn().mockResolvedValue({
      id: "key-relocated",
      relativePath: "id_build",
      group: "",
      files: [
        { from: "id_work", to: "id_build" },
        { from: "id_work.pub", to: "id_build.pub" },
      ],
      references: [
        {
          directive: "IdentityFile",
          configPath: "/home/.ssh/config",
          line: 2,
          from: "~/.ssh/id_work",
          to: "~/.ssh/id_build",
        },
      ],
      skipped: [],
      notes: [],
      blockers: [],
      transactionId: "tx",
    }),
    trash: vi.fn().mockResolvedValue({ entryId: "entry-1", files: [], skipped: [], transactionId: "tx" }),
    listTrash: vi.fn().mockResolvedValue({
      entries: [
        {
          id: "20260805T090000.000-aabbccdd",
          deletedAt: "2026-08-05T09:00:00Z",
          ageDays: 40,
          stale: true,
          files: [
            {
              originalRelativePath: "id_old",
              trashRelativePath: "sshc/trash/20260805T090000.000-aabbccdd/id_old",
              kind: "private_key",
              fingerprint: "SHA256:012345",
              permission: "0600",
            },
          ],
          restorable: true,
          blockers: [],
        },
      ],
      retentionDays: 30,
    }),
    restore: vi.fn().mockResolvedValue({
      entryId: "20260805T090000.000-aabbccdd",
      restored: ["id_old"],
      blockers: [],
      transactionId: "tx",
    }),
    purge: vi.fn().mockResolvedValue({
      entryId: "20260805T090000.000-aabbccdd",
      removed: ["id_old"],
      transactionId: "tx",
    }),
    ...overrides,
  };
}

describe("KeysScreen", () => {
  it("links directly to key creation and offers safe next steps after in-process generation", async () => {
    const api = buildApi();
    const onAssignGeneratedKey = vi.fn();
    const onInstallGeneratedKey = vi.fn();
    render(
      <KeysScreen
        api={api}
        onAssignGeneratedKey={onAssignGeneratedKey}
        onInstallGeneratedKey={onInstallGeneratedKey}
      />,
    );

    expect(await screen.findByRole("link", { name: "Create a key" })).toHaveAttribute("href", "#create-key-heading");
    await userEvent.type(screen.getByLabelText("File name"), "id_new");
    await userEvent.type(screen.getByLabelText("Passphrase"), "one-time secret");
    await userEvent.click(screen.getByRole("button", { name: "Create key" }));

    await userEvent.click(await screen.findByRole("button", { name: "Assign to a connection" }));
    expect(onAssignGeneratedKey).toHaveBeenCalledWith({
      privateKeyId: "key-new",
      privateRelativePath: "id_new",
    });
    await userEvent.click(screen.getByRole("button", { name: "Install on a server" }));
    expect(onInstallGeneratedKey).toHaveBeenCalledWith({ publicRelativePath: "id_new.pub" });
  });

  it("lists classified files with the marks that stop a scan", async () => {
    render(<KeysScreen api={buildApi()} />);

    const inventoryTable = await screen.findByRole("table", {
      name: "Files classified by content and permissions",
    });
    expect(inventoryTable).toHaveClass("block", "w-full", "md:table", "md:min-w-[56rem]");
    expect(inventoryTable).not.toHaveClass("min-w-[56rem]");

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    expect(workRow).toHaveClass("grid", "md:table-row");
    expect(within(workRow).getByText("referenced by 1")).toBeInTheDocument();
    expect(within(workRow).getByText("SHA256:abcdef")).toBeInTheDocument();

    const legacyRow = screen.getByRole("row", { name: /legacy/ });
    expect(within(legacyRow).getByText("Permissions too open")).toBeInTheDocument();
    expect(within(legacyRow).getByText("Fingerprint unavailable")).toBeInTheDocument();
  });

  it("sorts the key inventory from every data-column header", async () => {
    render(<KeysScreen api={buildApi()} />);

    const table = await screen.findByRole("table", { name: "Files classified by content and permissions" });
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("id_work");
    await userEvent.click(within(table).getByRole("button", { name: /File.*sort descending/ }));
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("legacy");
    await userEvent.click(within(table).getByRole("button", { name: /Kind.*sort ascending/ }));
    expect(within(table).getByRole("columnheader", { name: /Kind/ })).toHaveAttribute("aria-sort", "ascending");
    await userEvent.click(within(table).getByRole("button", { name: /State.*sort ascending/ }));
    expect(within(table).getByRole("columnheader", { name: /State/ })).toHaveAttribute("aria-sort", "ascending");
  });

  it("stacks key creation fields at phone widths", async () => {
    render(<KeysScreen api={buildApi()} />);

    const fileName = await screen.findByLabelText("File name");
    const row = fileName.closest("label")?.parentElement;
    expect(row).toHaveClass("flex-col", "items-stretch", "sm:flex-row", "sm:items-center");
    expect(fileName).toHaveClass("min-h-10", "sm:min-h-0");
  });

  it("hands the chosen key to the inspector, and takes it back", async () => {
    const user = userEvent.setup();
    const onInspector = vi.fn();
    render(<KeysScreen api={buildApi()} onInspector={onInspector} />);

    await user.click(await screen.findByRole("button", { name: "id_work" }));
    expect(onInspector).toHaveBeenLastCalledWith(
      expect.objectContaining({ label: "Key details", attention: false }),
    );

    await user.click(screen.getByRole("button", { name: "id_work" }));
    expect(onInspector).toHaveBeenLastCalledWith(null);
  });

  it("marks the inspector when the chosen key has open permissions", async () => {
    const user = userEvent.setup();
    const onInspector = vi.fn();
    render(<KeysScreen api={buildApi()} onInspector={onInspector} />);

    await user.click(await screen.findByRole("button", { name: "legacy" }));
    expect(onInspector).toHaveBeenLastCalledWith(expect.objectContaining({ attention: true }));
  });

  it("summarises the inventory and searches file, host and fingerprint", async () => {
    const user = userEvent.setup();
    render(<KeysScreen api={buildApi()} />);

    expect((await screen.findByText("Classified SSH files")).parentElement).toHaveTextContent("2");
    const search = screen.getByRole("searchbox", { name: "Search keys" });
    await user.type(search, "build-*");
    expect(screen.getByRole("row", { name: /id_work/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /legacy/ })).not.toBeInTheDocument();

    await user.clear(search);
    await user.type(search, "sha256:abcdef");
    expect(screen.getByRole("row", { name: /id_work/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /legacy/ })).not.toBeInTheDocument();
  });

  it("shows the exact ssh-keygen command for a hardware method instead of generating", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    await screen.findByRole("row", { name: /id_work/ });
    await userEvent.selectOptions(screen.getByLabelText("Algorithm"), "ed25519-sk");
    await userEvent.type(screen.getByLabelText("File name"), "id_yubikey");
    await userEvent.click(screen.getByRole("button", { name: "Show Terminal command" }));

    expect(await screen.findByLabelText("Terminal command")).toHaveTextContent(
      "ssh-keygen -t ed25519-sk -f /home/.ssh/id_yubikey",
    );
    expect(api.generate).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Assign to a connection" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Install on a server" })).toBeNull();
  });

  it("changes a passphrase and keeps nothing in the form afterwards", async () => {
    const changePassphrase = vi.fn().mockResolvedValue({
      id: "key-one",
      relativePath: "id_work",
      encrypted: true,
      notes: [],
      transactionId: "tx",
    });
    const api = buildApi({ changePassphrase });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const management = await openKeyDetails(workRow);
    expect(management.closest("tr")).toHaveAttribute("data-key-detail-for", "key-one");
    expect(management).toHaveClass("flex", "flex-wrap");
    await userEvent.click(within(management).getByRole("button", { name: "Change passphrase" }));

    await userEvent.type(screen.getByLabelText("Current passphrase"), "first passphrase");
    await userEvent.type(screen.getByLabelText("New passphrase"), "second passphrase");
    await userEvent.click(screen.getByRole("button", { name: "Save new passphrase" }));

    await waitFor(() =>
      expect(changePassphrase).toHaveBeenCalledWith("key-one", {
        currentPassphrase: "first passphrase",
        newPassphrase: "second passphrase",
        unencrypted: false,
      }),
    );
    await waitFor(() => expect(screen.queryByLabelText("Current passphrase")).not.toBeInTheDocument());

    const reopened = (await screen.findByRole("button", { name: "id_work" })).closest("tr")!;
    const reopenedActions = await openKeyDetails(reopened);
    await userEvent.click(within(reopenedActions).getByRole("button", { name: "Change passphrase" }));
    expect(screen.getByLabelText("Current passphrase")).toHaveValue("");
    expect(screen.getByLabelText("New passphrase")).toHaveValue("");
  });

  it("keeps a typed passphrase out of browser storage", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    render(<KeysScreen api={buildApi()} />);

    await screen.findByRole("row", { name: /id_work/ });
    await userEvent.type(screen.getByLabelText("File name"), "id_new");
    await userEvent.type(screen.getByLabelText("Passphrase"), "correct horse");

    expect(setItem).not.toHaveBeenCalled();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it("keeps the trash folded but says how much is in it", async () => {
    render(<KeysScreen api={buildApi()} />);

    const summary = await screen.findByText("Trash (1)");
    expect(summary.closest("details")).not.toHaveAttribute("open");
  });

  it("requires a second confirmation before a permanent delete", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    expect(api.purge).not.toHaveBeenCalled();

    expect(within(trashRow).getByText(/cannot be undone/)).toBeInTheDocument();
    await userEvent.click(within(trashRow).getByRole("button", { name: "Confirm permanent delete" }));
    await waitFor(() => expect(api.purge).toHaveBeenCalledWith("20260805T090000.000-aabbccdd"));
  });

  it("leaves the trash entry intact when the second confirmation is cancelled", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    await userEvent.click(within(trashRow).getByRole("button", { name: "Cancel" }));

    expect(api.purge).not.toHaveBeenCalled();
    expect(within(trashRow).getByRole("button", { name: "Delete permanently" })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /id_old/ })).toBeInTheDocument();
  });

  it("shows a refused restore as blockers instead of guessing", async () => {
    const api = buildApi({
      restore: vi.fn().mockResolvedValue({
        entryId: "20260805T090000.000-aabbccdd",
        restored: [],
        blockers: ["restore_path_occupied:id_old"],
        transactionId: "",
      }),
    });
    render(<KeysScreen api={api} />);

    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Restore" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("restore_path_occupied:id_old");
    expect(screen.getByRole("row", { name: /id_old/ })).toBeInTheDocument();
  });

  it("marks a trash entry older than the retention window without deleting it", async () => {
    render(<KeysScreen api={buildApi()} />);

    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    expect(within(trashRow).getByText("40 days · older than 30 days")).toBeInTheDocument();
    expect(within(trashRow).getByRole("button", { name: "Restore" })).toBeEnabled();
  });

  it("reports an unreadable ssh directory instead of an empty list", async () => {
    const api = buildApi({ inventory: vi.fn().mockRejectedValue(new Error("api_read_failed")) });
    render(<KeysScreen api={api} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be read");
  });

  it("names the files it could not classify instead of leaving them out", async () => {
    const inventory = buildInventory();
    inventory.unreadable = [
      { relativePath: "sockets/agent.sock", reason: "not_a_regular_file" },
      { relativePath: "vault", reason: "permission_denied" },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    expect(await screen.findByText(/sockets\/agent\.sock — not_a_regular_file/)).toBeInTheDocument();
    expect(screen.getByText(/vault — permission_denied/)).toBeInTheDocument();
    expect(screen.getByText(/missing from the table above/)).toBeInTheDocument();
  });

  it("reports a configuration entry pointing at a key that is not there", async () => {
    const inventory = buildInventory();
    inventory.unresolvedReferences = [
      {
        directive: "IdentityFile",
        value: "~/.ssh/id_gone",
        configPath: "config",
        line: 14,
        reason: "file_missing",
      },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    expect(
      await screen.findByText(/IdentityFile ~\/\.ssh\/id_gone — config:14 \(file_missing\)/),
    ).toBeInTheDocument();
  });

  it("shows a certificate's principals and marks one that has run out", async () => {
    const inventory = buildInventory();
    inventory.items = [
      {
        ...inventory.items[0]!,
        id: "cert-one",
        relativePath: "id_work-cert.pub",
        kind: "certificate",
        certificate: {
          keyId: "aida@dubguild",
          principals: ["deploy", "ops"],
          validBefore: 1577836800,
          neverExpires: false,
          signedKeyType: "ssh-ed25519",
          signedKeyFingerprint: "SHA256:signedkey",
        },
      },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    const row = await screen.findByRole("row", { name: /id_work-cert\.pub/ });
    expect(within(row).getByText("key id aida@dubguild")).toBeInTheDocument();
    expect(within(row).getByText("for deploy, ops")).toBeInTheDocument();
    expect(within(row).getByText("expired 2020-01-01 00:00Z")).toBeInTheDocument();
    expect(within(row).getByText("signs ssh-ed25519 SHA256:signedkey")).toBeInTheDocument();
  });

  it("says a certificate with no principal is valid for any of them", async () => {
    const inventory = buildInventory();
    inventory.items = [
      {
        ...inventory.items[0]!,
        id: "cert-two",
        relativePath: "host-cert.pub",
        kind: "certificate",
        certificate: {
          keyId: "host",
          principals: [],
          validBefore: 0,
          neverExpires: true,
          signedKeyType: "ssh-ed25519",
          signedKeyFingerprint: "SHA256:hostkey",
        },
      },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    const row = await screen.findByRole("row", { name: /host-cert\.pub/ });
    expect(within(row).getByText("for any principal")).toBeInTheDocument();
    expect(within(row).getByText("never expires")).toBeInTheDocument();
  });

  it("refuses agent registration, and says what is missing, when no agent is reachable", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    expect(within(actions).getByRole("button", { name: "Add to ssh-agent" })).toBeDisabled();
    expect(screen.getByText(/This process cannot connect to ssh-agent/)).toBeInTheDocument();
    expect(api.registerWithAgent).not.toHaveBeenCalled();
  });

  it("registers a key with the agent and the lifetime the user chose", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));

    await userEvent.type(screen.getByLabelText("Key passphrase"), "correct horse");
    await userEvent.selectOptions(screen.getByLabelText("Lifetime"), "3600");
    await userEvent.click(screen.getByRole("button", { name: "Add key to ssh-agent" }));

    await waitFor(() =>
      expect(api.registerWithAgent).toHaveBeenCalledWith("key-one", {
        passphrase: "correct horse",
        lifetimeSeconds: 3600,
      }),
    );
    await waitFor(() => expect(screen.queryByLabelText("Key passphrase")).not.toBeInTheDocument());
    expect(document.body).not.toHaveTextContent("correct horse");
  });

  it("keeps no passphrase after the agent refuses the key", async () => {
    const api = buildApi({
      inventory: vi.fn().mockResolvedValue(inventoryWithAgent()),
      registerWithAgent: vi.fn().mockRejectedValue(new Error("api_mutation_failed")),
    });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));
    await userEvent.type(screen.getByLabelText("Key passphrase"), "wrong passphrase");
    await userEvent.click(screen.getByRole("button", { name: "Add key to ssh-agent" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be added to ssh-agent");
    expect(screen.getByLabelText("Key passphrase")).toHaveValue("");
    expect(document.body).not.toHaveTextContent("wrong passphrase");
  });

  it("asks for no passphrase for a key that is not encrypted", async () => {
    const inventory = inventoryWithAgent();
    inventory.items = inventory.items.map((item) => ({ ...item, encrypted: false }));
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventory) });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));

    expect(screen.queryByLabelText("Key passphrase")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Add key to ssh-agent" }));

    await waitFor(() =>
      expect(api.registerWithAgent).toHaveBeenCalledWith("key-one", {
        passphrase: "",
        lifetimeSeconds: 0,
      }),
    );
  });

  it("shows what the agent holds and which entries delegate to it", async () => {
    const inventory = inventoryWithAgent();
    inventory.agentIdentities = [
      { bits: 256, fingerprint: "SHA256:heldbyagent", comment: "aida@laptop", algorithm: "ED25519" },
    ];
    inventory.agentDelegations = [
      {
        directive: "IdentityAgent",
        configPath: "config",
        line: 9,
        condition: "Host deploy",
        hostPatterns: ["deploy"],
        value: "SSH_AUTH_SOCK",
      },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    const identityRow = await screen.findByRole("row", { name: /SHA256:heldbyagent/ });
    expect(within(identityRow).getByText("ED25519 · 256")).toBeInTheDocument();
    expect(screen.getByText(/IdentityAgent SSH_AUTH_SOCK — config:9/)).toBeInTheDocument();
  });
  it("shows a public key and copies exactly the text it showed", async () => {
    const user = userEvent.setup();
    const inventory = buildInventory();
    inventory.items = [
      { ...inventory.items[0]!, id: "key-three", relativePath: "id_work.pub", kind: "public_key" },
    ];
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventory) });
    render(<KeysScreen api={api} />);

    const row = await screen.findByRole("row", { name: /id_work\.pub/ });
    const actions = await openKeyDetails(row);
    expect(within(actions).queryByRole("button", { name: "Show private key" })).not.toBeInTheDocument();
    await user.click(within(actions).getByRole("button", { name: "Show public key" }));

    const shown = await screen.findByLabelText("Public key");
    expect(shown).toHaveTextContent("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmU aida@laptop");
    const detailRow = shown.closest("tr");
    expect(detailRow).toHaveAttribute("data-key-detail-for", "key-three");
    expect(detailRow?.closest("table")).toBe(row.closest("table"));
    expect(row.compareDocumentPosition(detailRow!) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    await user.click(screen.getByRole("button", { name: "Copy public key" }));

    expect(await navigator.clipboard.readText()).toBe(
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmU aida@laptop",
    );
  });

  it("places the private-key confirmation directly below its key row", async () => {
    const api = buildApi({
      reveal: vi.fn().mockResolvedValue({
        id: "key-one",
        relativePath: "id_work",
        privateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nfixture\n-----END OPENSSH PRIVATE KEY-----\n",
        encrypted: true,
        fingerprint: "SHA256:abcdef",
        transactionId: "tx",
      }),
    });
    render(<KeysScreen api={api} />);

    const row = await screen.findByRole("row", { name: /id_work\b/ });
    const actions = await openKeyDetails(row);
    await userEvent.click(within(actions).getByRole("button", { name: "Show private key" }));

    const dialog = screen.getByRole("dialog");
    const detailRow = dialog.closest("tr");
    expect(detailRow).toHaveAttribute("data-key-detail-for", "key-one");
    expect(detailRow?.closest("table")).toBe(row.closest("table"));
    expect(dialog).not.toHaveAttribute("aria-modal");
    await userEvent.click(within(dialog).getByRole("button", { name: "Show private key" }));
    expect(await within(dialog).findByLabelText("Private key")).toHaveTextContent("BEGIN OPENSSH PRIVATE KEY");
  });

  it("offers no public key read for a private key", async () => {
    render(<KeysScreen api={buildApi()} />);

    const row = await screen.findByRole("row", { name: /id_work\b/ });
    const actions = await openKeyDetails(row);
    expect(within(actions).queryByRole("button", { name: "Show public key" })).not.toBeInTheDocument();
  });

  it("reports a public key that could not be read", async () => {
    const user = userEvent.setup();
    const inventory = buildInventory();
    inventory.items = [
      { ...inventory.items[0]!, id: "key-three", relativePath: "id_work.pub", kind: "public_key" },
    ];
    const api = buildApi({
      inventory: vi.fn().mockResolvedValue(inventory),
      publicKey: vi.fn().mockRejectedValue(new Error("api_read_failed")),
    });
    render(<KeysScreen api={api} />);

    const row = await screen.findByRole("row", { name: /id_work\.pub/ });
    const actions = await openKeyDetails(row);
    await user.click(within(actions).getByRole("button", { name: "Show public key" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("public key could not be read");
    expect(screen.queryByLabelText("Public key")).not.toBeInTheDocument();
  });

  it("toggles public material below its private key while leaving a public-only key visible", async () => {
    const user = userEvent.setup();
    const inventory = buildInventory();
    const privateKey = inventory.items[0]!;
    inventory.items = [
      { ...privateKey, id: "paired-public", relativePath: "id_work.pub", kind: "public_key" },
      {
        ...privateKey,
        id: "standalone-public",
        relativePath: "colleague.pub",
        kind: "public_key",
        fingerprint: "SHA256:colleague",
      },
      privateKey,
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    const privateRow = (await screen.findByRole("button", { name: "id_work" })).closest("tr")!;
    const toggle = within(privateRow).getByRole("button", { name: "Public key files (1)" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("button", { name: "id_work.pub" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "colleague.pub" })).toBeVisible();

    await user.click(toggle);

    const pairedPublic = await screen.findByRole("button", { name: "id_work.pub" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(privateRow.compareDocumentPosition(pairedPublic.closest("tr")!) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);

    await user.click(toggle);
    expect(screen.queryByRole("button", { name: "id_work.pub" })).not.toBeInTheDocument();
  });

  it("relocates a key and reports what moved and what it rewrote", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const row = await screen.findByRole("row", { name: /id_work\b/ });
    const actions = await openKeyDetails(row);
    await user.click(within(actions).getByRole("button", { name: "Rename or move" }));

    const field = screen.getByLabelText("Name");
    expect(field).toHaveValue("id_work");
    await user.clear(field);
    await user.type(field, "id_build");
    await user.click(screen.getByRole("button", { name: "Rename or move the key" }));

    await waitFor(() => expect(api.relocate).toHaveBeenCalledWith("key-one", { newName: "id_build" }));
    expect(await screen.findByText(/IdentityFile ~\/\.ssh\/id_work → ~\/\.ssh\/id_build/)).toBeInTheDocument();
    expect(screen.getByText("id_work.pub → id_build.pub")).toBeInTheDocument();
  });

  it("keeps a blocked relocation on screen with the reasons it refused", async () => {
    const user = userEvent.setup();
    const api = buildApi({
      relocate: vi.fn().mockResolvedValue({
        id: "",
        relativePath: "",
        group: "",
        files: [],
        references: [],
        skipped: [],
        notes: [],
        blockers: ["key_reference_unresolved:id_work", "key_destination_occupied:id_build"],
        transactionId: "",
      }),
    });
    render(<KeysScreen api={api} />);

    const row = await screen.findByRole("row", { name: /id_work\b/ });
    const actions = await openKeyDetails(row);
    await user.click(within(actions).getByRole("button", { name: "Rename or move" }));
    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "id_build");
    await user.click(screen.getByRole("button", { name: "Rename or move the key" }));

    const reasons = await screen.findByRole("alert");
    expect(reasons).toHaveTextContent("cannot resolve");
    expect(reasons).toHaveTextContent("id_build already exists");
    expect(api.inventory).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText("Name")).toHaveValue("id_build");
  });

  it("offers no relocation for half of a key pair", async () => {
    const inventory = buildInventory();
    inventory.items = [
      inventory.items[0]!,
      { ...inventory.items[0]!, id: "key-three", relativePath: "id_work.pub", kind: "public_key" },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    const privateRow = (await screen.findByRole("button", { name: "id_work" })).closest("tr")!;
    await userEvent.click(within(privateRow).getByRole("button", { name: "Public key files (1)" }));
    const publicRow = await screen.findByRole("row", { name: /id_work\.pub/ });
    const publicActions = await openKeyDetails(publicRow);
    expect(within(publicActions).queryByRole("button", { name: "Rename or move" })).not.toBeInTheDocument();
    const privateActions = await openKeyDetails(privateRow);
    expect(within(privateActions).getByRole("button", { name: "Rename or move" })).toBeInTheDocument();
  });

  it("offers relocation for a public key that stands alone, starting from its stem", async () => {
    const user = userEvent.setup();
    const inventory = buildInventory();
    inventory.items = [
      {
        ...inventory.items[0]!,
        id: "key-three",
        relativePath: "colleague.pub",
        kind: "public_key",
        fingerprint: "SHA256:someone-else",
      },
    ];
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} />);

    const row = await screen.findByRole("row", { name: /colleague\.pub/ });
    const actions = await openKeyDetails(row);
    await user.click(within(actions).getByRole("button", { name: "Rename or move" }));

    expect(screen.getByLabelText("Name")).toHaveValue("colleague");
  });

  it("gives every form control a visible style", async () => {
    const { container } = render(<KeysScreen api={buildApi()} groups={["work"]} />);
    await screen.findByRole("row", { name: /id_work/ });

    const controls = container.querySelectorAll("input, select, textarea");
    expect(controls.length).toBeGreaterThan(0);
    for (const element of controls) {
      if (element instanceof HTMLInputElement && ["checkbox", "color"].includes(element.type)) continue;
      expect(element.className, `${element.tagName} ${element.getAttribute("value") ?? ""} has no style`).not.toBe("");
    }
  });
  it("says what a trash move takes with it, and which hosts still name the key", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const privateRow = (await screen.findByRole("button", { name: "id_work" })).closest("tr")!;
    const actions = await openKeyDetails(privateRow);
    await user.click(within(actions).getByRole("button", { name: "Move to trash" }));

    expect(await screen.findByText(/These files move together/)).toBeInTheDocument();
    expect(screen.getByText(/Nothing is deleted/)).toBeInTheDocument();
    expect(api.trash).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Move it to the trash" }));
    await waitFor(() => expect(api.trash).toHaveBeenCalled());
  });

  it("stores a key passphrase from the private key row and assigns it without retaining the value", async () => {
    const user = userEvent.setup();
    const secrets = buildSecrets();
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    await user.type(screen.getByLabelText("Passphrase name"), "production-key");
    await user.type(screen.getByLabelText("Passphrase value"), "a secret phrase only for this test");
    await user.click(screen.getByRole("button", { name: "Save and use for this key" }));

    await waitFor(() =>
      expect(secrets.storeCredential).toHaveBeenCalledWith(
        "key_passphrase",
        "production-key",
        "a secret phrase only for this test",
      ),
    );
    expect(secrets.assignCredential).toHaveBeenCalledWith("key_passphrase", "id_work", "production-key");
    expect(document.body).not.toHaveTextContent("a secret phrase only for this test");
    expect(screen.queryByLabelText("Passphrase value")).not.toBeInTheDocument();
  });

  it("detaches a stored passphrase without deleting the named credential", async () => {
    const user = userEvent.setup();
    const assigned = {
      credentials: [{ kind: "key_passphrase", name: "build-key", uses: ["id_work"] }],
    };
    const secrets = buildSecrets({
      credentials: vi.fn().mockResolvedValue(assigned),
      unassignCredential: vi.fn().mockResolvedValue({
        credentials: [{ kind: "key_passphrase", name: "build-key", uses: [] }],
      }),
    });
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    expect(await screen.findByText(/uses the stored passphrase named build-key/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Stop using it" }));

    await waitFor(() =>
      expect(secrets.unassignCredential).toHaveBeenCalledWith("key_passphrase", "id_work"),
    );
    expect(screen.getByRole("option", { name: "build-key" })).toBeInTheDocument();
  });

  it("reports a dedicated passphrase as saved only for this key", async () => {
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: ["id_work"],
        minPassphraseLength: 12,
      }),
    });
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    expect(secrets.passwordVault).not.toHaveBeenCalled();
    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);

    expect(await screen.findByText("A passphrase is saved only for this key.")).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent("a dedicated secret value");
  });

  it("detaches a dedicated passphrase through the same key-owned removal", async () => {
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: ["id_work"],
        minPassphraseLength: 12,
      }),
    });
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    await userEvent.click(await screen.findByRole("button", { name: "Stop using it" }));

    await waitFor(() =>
      expect(secrets.unassignCredential).toHaveBeenCalledWith("key_passphrase", "id_work"),
    );
    expect(screen.queryByText("A passphrase is saved only for this key.")).not.toBeInTheDocument();
  });

  it("replaces only this key's dedicated value when a named passphrase is selected", async () => {
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: ["id_work"],
        minPassphraseLength: 12,
      }),
      assignCredential: vi.fn().mockResolvedValue({
        credentials: [{ kind: "key_passphrase", name: "build-key", uses: ["id_work"] }],
      }),
    });
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    expect(await screen.findByText("A passphrase is saved only for this key.")).toBeInTheDocument();
    await userEvent.selectOptions(await screen.findByLabelText("Use a stored passphrase"), "build-key");
    await userEvent.click(screen.getByRole("button", { name: "Use this passphrase" }));

    expect(await screen.findByText(/uses the stored passphrase named build-key/)).toBeInTheDocument();
    expect(screen.queryByText("A passphrase is saved only for this key.")).not.toBeInTheDocument();
  });

  it("does not rotate a shared passphrase from the create-and-assign form", async () => {
    const user = userEvent.setup();
    const secrets = buildSecrets();
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    await user.type(screen.getByLabelText("Passphrase name"), "build-key");
    await user.type(screen.getByLabelText("Passphrase value"), "must not replace the shared value");
    await user.click(screen.getByRole("button", { name: "Save and use for this key" }));

    expect(secrets.storeCredential).not.toHaveBeenCalled();
    expect(secrets.assignCredential).not.toHaveBeenCalled();
    expect(screen.getByText(/passphrase with this name already exists/)).toBeInTheDocument();
    expect(screen.getByLabelText("Passphrase value")).toHaveValue("");
  });

});

describe("taking a key back out of the agent", () => {
  it("removes the identity the agent is holding for this key", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithLoadedKey()) });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Remove from ssh-agent" }));

    await waitFor(() => expect(api.deregisterFromAgent).toHaveBeenCalledWith("key-one"));
  });

  it("offers nothing to remove when the agent is not holding this key", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    expect(within(actions).queryByRole("button", { name: "Remove from ssh-agent" })).not.toBeInTheDocument();
  });

  it("offers a stored passphrase for the key, and never an account password", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    render(<KeysScreen api={api} secrets={buildSecrets()} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));

    const picker = await screen.findByLabelText("Use a stored passphrase");
    expect(picker).toHaveTextContent("build-key");
    expect(picker).not.toHaveTextContent("office-vm");
  });

  it("points a key at a stored passphrase, so adding it is one action", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    const secrets = buildSecrets();
    render(<KeysScreen api={api} secrets={secrets} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));
    await userEvent.selectOptions(await screen.findByLabelText("Use a stored passphrase"), "build-key");
    await userEvent.click(screen.getByRole("button", { name: "Use this passphrase" }));

    await waitFor(() =>
      expect(secrets.assignCredential).toHaveBeenCalledWith("key_passphrase", "id_work", "build-key"),
    );
  });

  it("says the stored passphrase will be used when the field is left empty", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    const secrets = buildSecrets({
      credentials: vi.fn().mockResolvedValue({
        credentials: [{ kind: "key_passphrase", name: "build-key", uses: ["id_work"] }],
      }),
    });
    render(<KeysScreen api={api} secrets={secrets} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));

    expect(await screen.findByText(/uses the stored passphrase named build-key/)).toBeInTheDocument();
    expect(screen.getByLabelText("Key passphrase")).toBeEnabled();
  });

  it("uses a key-dedicated saved passphrase for agent registration without exposing it", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: ["id_work"],
        minPassphraseLength: 12,
      }),
    });
    render(<KeysScreen api={api} secrets={secrets} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyDetails(workRow);
    await userEvent.click(within(actions).getByRole("button", { name: "Add to ssh-agent" }));

    expect(await screen.findByText("A passphrase is saved only for this key.")).toBeInTheDocument();
    expect(screen.getByText(/Leave this empty to use the saved passphrase/)).toBeInTheDocument();
    expect(screen.getByLabelText("Key passphrase")).toHaveValue("");
    expect(document.body).not.toHaveTextContent("a dedicated secret value");
  });
});

describe("organising keys into folders", () => {
  function grouped(): KeyInventoryResponse {
    const inventory = buildInventory();
    inventory.items[0]!.relativePath = "keys/work/id_work";
    inventory.items[1]!.relativePath = "id_loose";
    return inventory;
  }

  it("shows only the keys inside the folder that is open", async () => {
    const user = userEvent.setup();
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(grouped()) })} groups={["work"]} />);

    expect(await screen.findByRole("row", { name: /id_work/ })).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /id_loose/ })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "work, 1" }));

    expect(screen.getByRole("row", { name: /id_work/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /id_loose/ })).not.toBeInTheDocument();
  });

  it("moves every key that was chosen in one action", async () => {
    const user = userEvent.setup();
    const relocate = vi.fn().mockResolvedValue({
      id: "",
      relativePath: "",
      group: "work",
      files: [],
      references: [],
      skipped: [],
      notes: [],
      blockers: [],
      transactionId: "tx",
    });
    render(
      <KeysScreen
        api={buildApi({ inventory: vi.fn().mockResolvedValue(grouped()), relocate })}
        groups={["work"]}
      />,
    );

    await user.click(await screen.findByRole("checkbox", { name: "Choose id_loose" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Move into" }), "work");
    await user.click(screen.getByRole("button", { name: "Move" }));

    expect(relocate).toHaveBeenCalledWith("key-two", { group: "work" });
    expect(await screen.findByText("Moved 1.")).toBeInTheDocument();
  });

  it("names the key that was refused and still moves the rest", async () => {
    const user = userEvent.setup();
    const bothInWork = (): KeyInventoryResponse => {
      const inventory = buildInventory();
      inventory.items[0]!.relativePath = "keys/work/id_work";
      inventory.items[1]!.relativePath = "keys/work/legacy";
      return inventory;
    };
    const relocate = vi.fn(async (keyId: string) => ({
      id: keyId,
      relativePath: "",
      group: "",
      files: [],
      references: [],
      skipped: [],
      notes: [],
      blockers: keyId === "key-two" ? ["an Include glob would read the destination as configuration"] : [],
      transactionId: "tx",
    }));
    render(
      <KeysScreen
        api={buildApi({ inventory: vi.fn().mockResolvedValue(bothInWork()), relocate })}
        groups={["work"]}
      />,
    );

    await user.click(await screen.findByRole("checkbox", { name: "Choose keys/work/id_work" }));
    await user.click(screen.getByRole("checkbox", { name: "Choose keys/work/legacy" }));
    await user.click(screen.getByRole("button", { name: "Move" }));

    expect(await screen.findByText("Moved 1.")).toBeInTheDocument();
    expect(
      screen.getByText(/keys\/work\/legacy cannot be moved: an Include glob would read the destination as configuration/),
    ).toBeInTheDocument();
  });
});

describe("dragging a key onto a folder", () => {
  it("chooses what was grabbed and moves it where it was dropped", async () => {
    const relocate = vi.fn().mockResolvedValue({
      id: "",
      relativePath: "",
      group: "archive",
      files: [],
      references: [],
      skipped: [],
      notes: [],
      blockers: [],
      transactionId: "tx",
    });
    render(<KeysScreen api={buildApi({ relocate })} groups={["archive"]} />);

    fireEvent.dragStart(await screen.findByLabelText("Drag id_work"), {
      dataTransfer: { setData: vi.fn(), effectAllowed: "" },
    });
    fireEvent.drop(screen.getByRole("button", { name: "archive, 0" }));

    await waitFor(() => expect(relocate).toHaveBeenCalledWith("key-one", { group: "archive" }));
  });

  it("opening one form from a row closes the one that was open", async () => {
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: [],
        minPassphraseLength: 12,
      }),
    });
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    expect(await screen.findByRole("heading", { name: /Saved passphrase/ })).toBeInTheDocument();

    const actions = await openKeyDetails(row);
    await userEvent.click(within(actions).getByRole("button", { name: "Change passphrase" }));

    expect(await screen.findByLabelText("Current passphrase")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Saved passphrase/ })).toBeNull();
  });
});

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { vaultStatus } from "../testing/vaultStatus";
import type { CreateConnectionRequest, CreateConnectionResponse, Overview } from "../api/config";
import type { KeyInventoryResponse, KeysApi } from "../keys/api";
import { maxAliasLength } from "../rules/generated";
import { CreateConnectionPane } from "./CreateConnectionPane";
import type { CreateConnectionDraft, CreationPrerequisite } from "./createConnectionDraft";
import type { ConnectionSecretsApi } from "./secretsApi";

const privateKey = {
  id: "0123456789abcdef0123456789abcdef",
  relativePath: "id_work",
  kind: "private_key",
  container: "OPENSSH PRIVATE KEY",
  algorithm: "ed25519",
  keyType: "ssh-ed25519",
  bits: 256,
  encrypted: true,
  fingerprint: "SHA256:private",
  comment: "aida@laptop",
  permission: "0600",
  permissionRisk: false,
  sizeBytes: 444,
  references: [],
  notes: [],
};

const inventory: KeyInventoryResponse = {
  items: [
    privateKey,
    { ...privateKey, id: "fedcba9876543210fedcba9876543210", relativePath: "id_work.pub", kind: "public_key" },
  ],
  unreadable: [],
  agentDelegations: [],
  unresolvedReferences: [],
  agentAvailable: false,
  agentIdentities: [],
};

const groups: Overview["groups"] = [
  {
    name: "home-lab",
    directory: "connections/home-lab",
    keyDirectory: "keys/home-lab",
    settings: [{ keyword: "User", values: ["lab-admin"] }, { keyword: "Port", values: ["2202"] }],
    memberCount: 0,
    directoryPresent: true,
  },
  {
    name: "home-lab/others",
    parent: "home-lab",
    directory: "connections/home-lab/others",
    keyDirectory: "keys/home-lab/others",
    memberCount: 0,
    directoryPresent: false,
  },
];

const created: CreateConnectionResponse = {
  transactionId: "tx-create",
  identity: { path: "connections/home-lab/others/lab-node.conf", alias: "lab-node" },
  preview: { operation: "connection.create", diffs: [] },
};

type PaneOverrides = {
  createConnection?: (request: CreateConnectionRequest) => Promise<CreateConnectionResponse>;
  inventory?: KeysApi["inventory"];
  passwordVault?: ConnectionSecretsApi["passwordVault"];
  credentials?: ConnectionSecretsApi["credentials"];
  initialiseVault?: ConnectionSecretsApi["initialiseVault"];
  unlockVault?: ConnectionSecretsApi["unlockVault"];
  groups?: Overview["groups"];
  initialDraft?: CreateConnectionDraft;
  onOpenPrerequisite?: (section: CreationPrerequisite, draft: CreateConnectionDraft) => void;
  onCancel?: () => void;
  onCreated?: (result: CreateConnectionResponse) => void;
  onDirtyChange?: (dirty: boolean) => void;
};

function renderPane(overrides: PaneOverrides = {}) {
  const createConnection = overrides.createConnection ?? vi.fn().mockResolvedValue(created);
  const keyInventory = overrides.inventory ?? vi.fn().mockResolvedValue(inventory);
  const passwordVault = overrides.passwordVault ?? vi.fn().mockResolvedValue(vaultStatus());
  const credentials = overrides.credentials ?? vi.fn().mockResolvedValue({
    credentials: [
      { kind: "password", name: "office", uses: ["bastion"] },
      { kind: "key_passphrase", name: "id_work", uses: ["id_work"] },
    ],
  });
  const initialiseVault = overrides.initialiseVault ?? vi.fn().mockResolvedValue(vaultStatus());
  const unlockVault = overrides.unlockVault ?? vi.fn().mockResolvedValue(vaultStatus());
  const onCancel = overrides.onCancel ?? vi.fn();
  const onCreated = overrides.onCreated ?? vi.fn();
  const onOpenPrerequisite = vi.fn(overrides.onOpenPrerequisite ?? (() => undefined));
  const onCreateGroup = vi.fn().mockResolvedValue(true);
  const onDirtyChange = overrides.onDirtyChange ?? vi.fn();

  const rendered = render(
    <CreateConnectionPane
      groups={overrides.groups ?? groups}
      initialDraft={overrides.initialDraft}
      config={{ createConnection }}
      keys={{ inventory: keyInventory }}
      secrets={{ passwordVault, credentials, initialiseVault, unlockVault }}
      onCancel={onCancel}
      onCreated={onCreated}
      onOpenPrerequisite={onOpenPrerequisite}
      onCreateGroup={onCreateGroup}
      onDirtyChange={onDirtyChange}
    />,
  );
  return {
    createConnection, keyInventory, passwordVault, credentials, initialiseVault, unlockVault,
    onCancel, onCreated, onOpenPrerequisite, onCreateGroup, onDirtyChange,
    unmount: rendered.unmount,
  };
}

async function chooseAuthentication(user: ReturnType<typeof userEvent.setup>, name: string) {
  const method = await screen.findByLabelText("Authentication method");
  await waitFor(() => expect(method).toBeEnabled());
  await user.selectOptions(method, screen.getByRole("option", { name }));
}

async function fillConnection(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Connection name"), "lab-node");
  await user.selectOptions(screen.getByLabelText("Group"), "home-lab/others");
  await user.type(screen.getByLabelText("Host name or IP address"), "2001:db8::1");
  await user.type(screen.getByLabelText("User"), "root");
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("CreateConnectionPane", () => {
  it("is a form in the page rather than a dialog, focuses the name, and lists an empty nested declared group", async () => {
    renderPane();

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("form", { name: "New connection" })).toBeInTheDocument();
    expect(screen.getByLabelText("Connection name")).toHaveFocus();
    expect(screen.getByLabelText("Group")).toHaveDisplayValue("No group");
    expect(screen.getByRole("option", { name: "home-lab/others" })).toBeInTheDocument();
    expect(screen.getByLabelText("Port")).toHaveValue(null);
    expect(screen.getByLabelText("Port")).toHaveAttribute("placeholder", "22");
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());
    expect(screen.getByLabelText("Authentication method")).toHaveDisplayValue("SSH private key");
  });

  it("shows the chosen group's defaults, inherited from its parent, in the empty fields", async () => {
    const user = userEvent.setup();
    renderPane();
    await user.selectOptions(screen.getByLabelText("Group"), "home-lab/others");

    expect(screen.getByLabelText("User")).toHaveAttribute("placeholder", "lab-admin");
    expect(screen.getByLabelText("Port")).toHaveAttribute("placeholder", "2202");
    expect(screen.getByText("Empty fields use the defaults of group home-lab/others.")).toBeInTheDocument();
  });

  it("tells the page it has a draft once something is typed", async () => {
    const user = userEvent.setup();
    const harness = renderPane();
    await waitFor(() => expect(harness.onDirtyChange).toHaveBeenLastCalledWith(false));
    await user.type(screen.getByLabelText("Connection name"), "l");
    expect(harness.onDirtyChange).toHaveBeenLastCalledWith(true);
  });

  it("submits blank Port without launching anything and clears its dedicated password on success", async () => {
    const user = userEvent.setup();
    const harness = renderPane();
    await fillConnection(user);
    await chooseAuthentication(user, "Encrypted password for this connection");
    await user.type(screen.getByLabelText("Connection password"), "connection-only");

    await user.click(screen.getByRole("button", { name: "Create connection" }));

    await waitFor(() => expect(harness.createConnection).toHaveBeenCalledWith({
      alias: "lab-node",
      group: "home-lab/others",
      hostName: "2001:db8::1",
      user: "root",
      authentication: { kind: "dedicated_password", password: "connection-only" },
    }));
    expect(harness.onCreated).toHaveBeenCalledWith(created);
    expect(screen.getByLabelText("Connection password")).toHaveValue("");
  });

  it("supports saved, new shared, and private-key authentication without showing key passphrases or public keys", async () => {
    const user = userEvent.setup();
    const harness = renderPane();
    await fillConnection(user);

    await chooseAuthentication(user, "Saved password");
    expect(await screen.findByRole("option", { name: "office" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "id_work" })).not.toBeInTheDocument();

    await chooseAuthentication(user, "New saved password");
    await user.type(screen.getByLabelText("Saved password name"), "lab-shared");
    await user.type(screen.getByLabelText("New password"), "shared-secret");
    await user.click(screen.getByRole("button", { name: "Create connection" }));
    await waitFor(() => expect(harness.createConnection).toHaveBeenCalledWith(expect.objectContaining({
      authentication: { kind: "new_shared_password", credential: "lab-shared", password: "shared-secret" },
    })));

    await chooseAuthentication(user, "SSH private key");
    expect(screen.getByRole("option", { name: /id_work/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /id_work\.pub/ })).not.toBeInTheDocument();
  });

  it("can initialise or unlock the vault before creation", async () => {
    const user = userEvent.setup();
    const missing = renderPane({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ exists: false, unlocked: false })),
    });
    await chooseAuthentication(user, "Encrypted password for this connection");
    await user.type(await screen.findByLabelText("Master password"), "a long master password");
    await user.type(screen.getByLabelText("Confirm master password"), "a long master password");
    await user.click(screen.getByRole("button", { name: "Create encrypted vault" }));
    await waitFor(() => expect(missing.initialiseVault).toHaveBeenCalledWith("a long master password"));

    missing.unmount();
    const locked = renderPane({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ unlocked: false })),
    });
    await chooseAuthentication(user, "Encrypted password for this connection");
    await user.type(await screen.findByLabelText("Master password"), "the master password");
    await user.click(screen.getByRole("button", { name: "Unlock vault" }));
    await waitFor(() => expect(locked.unlockVault).toHaveBeenCalledWith("the master password"));
  });

  it("allows private-key creation while the password vault is locked", async () => {
    const user = userEvent.setup();
    const harness = renderPane({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ unlocked: false })),
    });
    await fillConnection(user);
    await chooseAuthentication(user, "SSH private key");
    await user.click(screen.getByRole("button", { name: "Create connection" }));

    await waitFor(() => expect(harness.createConnection).toHaveBeenCalledWith(expect.objectContaining({
      authentication: { kind: "identity_file", keyId: privateKey.id },
    })));
    expect(harness.unlockVault).not.toHaveBeenCalled();
  });

  it("shows inline validation and a server problem and keeps the typed password for the retry", async () => {
    const user = userEvent.setup();
    const createConnection = vi.fn().mockRejectedValue(new Error("rejected"));
    renderPane({ createConnection });
    await chooseAuthentication(user, "Encrypted password for this connection");
    await user.type(screen.getByLabelText("Connection name"), "bad name");
    await user.tab();
    expect(screen.getByText("Use letters, numbers, dot, dash, or underscore; start with a letter or number.")).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Connection name"));
    await user.type(screen.getByLabelText("Connection name"), "good-name");
    await user.type(screen.getByLabelText("Host name or IP address"), "host.example");
    await user.type(screen.getByLabelText("Connection password"), "kept-for-retry");
    await user.click(screen.getByRole("button", { name: "Create connection" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The connection could not be created");
    expect(screen.getByLabelText("Connection password")).toHaveValue("kept-for-retry");
  });

  it("refuses a connection name longer than the alias limit the engine enforces", async () => {
    const user = userEvent.setup();
    renderPane();
    const aliasAtLimit = "a".repeat(maxAliasLength);
    await user.type(await screen.findByLabelText("Connection name"), aliasAtLimit);
    await user.tab();
    expect(screen.queryByText("Use letters, numbers, dot, dash, or underscore; start with a letter or number.")).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("Connection name"), "a");
    expect(screen.getByText("Use letters, numbers, dot, dash, or underscore; start with a letter or number.")).toBeInTheDocument();
  });

  it("keeps the master password after a refused unlock so it can be corrected in place", async () => {
    const user = userEvent.setup();
    const harness = renderPane({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ unlocked: false })),
      unlockVault: vi.fn().mockRejectedValue(new Error("wrong passphrase")),
    });
    await chooseAuthentication(user, "Encrypted password for this connection");
    await user.type(await screen.findByLabelText("Master password"), "a mistyped master");
    await user.click(screen.getByRole("button", { name: "Unlock vault" }));

    await waitFor(() => expect(harness.unlockVault).toHaveBeenCalledWith("a mistyped master"));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByLabelText("Master password")).toHaveValue("a mistyped master");
  });

  it("clears secrets on Cancel", async () => {
    const user = userEvent.setup();
    const cancel = renderPane();
    await chooseAuthentication(user, "Encrypted password for this connection");
    await user.type(await screen.findByLabelText("Connection password"), "cancel-secret");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(cancel.onCancel).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText("Connection password")).toHaveValue("");
  });

  it("opens the key screen with a resumable draft but never copies passwords into it", async () => {
    const user = userEvent.setup();
    const noKeys: KeyInventoryResponse = { ...inventory, items: [] };
    const harness = renderPane({
      groups: [],
      inventory: vi.fn().mockResolvedValue(noKeys),
    });

    await user.type(screen.getByLabelText("Connection name"), "lab-node");
    await user.type(screen.getByLabelText("Host name or IP address"), "host.example");
    await user.type(await screen.findByLabelText("Connection password"), "must-not-leave-the-form");
    await chooseAuthentication(user, "SSH private key");
    expect(screen.getByRole("option", { name: "No private keys available" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create a private key" }));

    expect(harness.onOpenPrerequisite).toHaveBeenCalledWith("Keys", expect.objectContaining({
      alias: "lab-node",
      hostName: "host.example",
      authentication: "identity_file",
    }));
    expect(JSON.stringify(harness.onOpenPrerequisite.mock.calls[0])).not.toContain("must-not-leave-the-form");
  });

  it("creates a group in place and chooses it, keeping what was typed", async () => {
    const user = userEvent.setup();
    const props = {
      config: { createConnection: vi.fn() },
      keys: { inventory: vi.fn().mockResolvedValue(inventory) },
      secrets: { passwordVault: vi.fn().mockResolvedValue(vaultStatus()), credentials: vi.fn().mockResolvedValue({ credentials: [] }), initialiseVault: vi.fn(), unlockVault: vi.fn() },
      onCancel: vi.fn(),
      onCreated: vi.fn(),
    };
    const withLab = [...groups, { ...groups[1]!, name: "lab", directory: "connections/lab", keyDirectory: "keys/lab", memberCount: 0 }];
    // The page reads the groups again after saving, so the new group is in the list when creation answers.
    const onCreateGroup = vi.fn(async () => {
      rendered.rerender(<CreateConnectionPane groups={withLab} onCreateGroup={onCreateGroup} {...props} />);
      return true;
    });
    const rendered = render(<CreateConnectionPane groups={groups} onCreateGroup={onCreateGroup} {...props} />);

    await user.type(screen.getByLabelText("Connection name"), "lab-node");
    await user.click(screen.getByRole("button", { name: "New group" }));
    await user.type(screen.getByLabelText("Name of the new group"), "lab/x");
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(screen.getByRole("alert")).toHaveTextContent("cannot contain “/”");
    expect(onCreateGroup).not.toHaveBeenCalled();

    await user.keyboard("{Escape}");
    expect(screen.queryByLabelText("Name of the new group")).toBeNull();
    expect(props.onCancel).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "New group" }));
    await user.type(screen.getByLabelText("Name of the new group"), "lab{Enter}");
    expect(onCreateGroup).toHaveBeenCalledWith("lab");
    expect(props.config.createConnection).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByLabelText("Group")).toHaveValue("lab"));
    expect(screen.queryByLabelText("Name of the new group")).toBeNull();
    expect(screen.getByLabelText("Connection name")).toHaveValue("lab-node");
  });

  it("restores a safe draft and explains why creation is disabled", async () => {
    renderPane({
      initialDraft: {
        alias: "lab-node",
        group: "home-lab/others",
        hostName: "host.example",
        user: "root",
        port: "2202",
        authentication: "dedicated_password",
        savedCredential: "office",
        newCredential: "",
        keyId: "",
      },
    });

    expect(screen.getByLabelText("Connection name")).toHaveValue("lab-node");
    expect(screen.getByLabelText("Group")).toHaveValue("home-lab/others");
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("host.example");
    expect(screen.getByLabelText("User")).toHaveValue("root");
    expect(screen.getByLabelText("Port")).toHaveValue(2202);
    expect(await screen.findByText("Enter a connection password to continue.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create connection" })).toBeDisabled();
  });
});

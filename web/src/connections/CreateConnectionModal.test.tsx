import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CreateConnectionRequest, CreateConnectionResponse, Overview } from "../api/config";
import type { IntegrationsApi } from "../api/integrations";
import type { KeyInventoryResponse, KeysApi } from "../keys/api";
import {
  CreateConnectionModal,
  type CreateConnectionDraft,
  type CreationPrerequisite,
} from "./CreateConnectionModal";

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

type ModalOverrides = {
  createConnection?: (request: CreateConnectionRequest) => Promise<CreateConnectionResponse>;
  inventory?: KeysApi["inventory"];
  passwordVault?: IntegrationsApi["passwordVault"];
  credentials?: IntegrationsApi["credentials"];
  initialiseVault?: IntegrationsApi["initialiseVault"];
  unlockVault?: IntegrationsApi["unlockVault"];
  groups?: Overview["groups"];
  initialDraft?: CreateConnectionDraft;
  onOpenPrerequisite?: (section: CreationPrerequisite, draft: CreateConnectionDraft) => void;
  onClose?: () => void;
  onCreated?: (result: CreateConnectionResponse) => void;
};

function renderModal(overrides: ModalOverrides = {}) {
  const createConnection = overrides.createConnection ?? vi.fn().mockResolvedValue(created);
  const keyInventory = overrides.inventory ?? vi.fn().mockResolvedValue(inventory);
  const passwordVault = overrides.passwordVault ?? vi.fn().mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  });
  const credentials = overrides.credentials ?? vi.fn().mockResolvedValue({
    credentials: [
      { kind: "password", name: "office", uses: ["bastion"] },
      { kind: "key_passphrase", name: "id_work", uses: ["id_work"] },
    ],
  });
  const initialiseVault = overrides.initialiseVault ?? vi.fn().mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  });
  const unlockVault = overrides.unlockVault ?? vi.fn().mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  });
  const onClose = overrides.onClose ?? vi.fn();
  const onCreated = overrides.onCreated ?? vi.fn();
  const onOpenPrerequisite = vi.fn(overrides.onOpenPrerequisite ?? (() => undefined));

  const rendered = render(
    <CreateConnectionModal
      groups={overrides.groups ?? groups}
      initialDraft={overrides.initialDraft}
      config={{ createConnection } as never}
      keys={{ inventory: keyInventory } as Pick<KeysApi, "inventory">}
      secrets={{ passwordVault, credentials, initialiseVault, unlockVault } as Pick<
        IntegrationsApi,
        "passwordVault" | "credentials" | "initialiseVault" | "unlockVault"
      >}
      onClose={onClose}
      onCreated={onCreated}
      onOpenPrerequisite={onOpenPrerequisite}
    />,
  );
  return {
    createConnection, keyInventory, passwordVault, credentials, initialiseVault, unlockVault,
    onClose, onCreated, onOpenPrerequisite,
    unmount: rendered.unmount,
  };
}

async function fillConnection(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Connection name"), "lab-node");
  await user.selectOptions(screen.getByLabelText("Save in group"), "home-lab/others");
  await user.type(screen.getByLabelText("Host name or IP address"), "2001:db8::1");
  await user.type(screen.getByLabelText("User (optional)"), "root");
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("CreateConnectionModal", () => {
  it("is a modal dialog, focuses the name, and lists an empty nested declared group", async () => {
    renderModal();

    expect(screen.getByRole("dialog", { name: "Create connection" })).toHaveAttribute("aria-modal", "true");
    expect(screen.getByLabelText("Connection name")).toHaveFocus();
    expect(screen.getByLabelText("Save in group")).toHaveDisplayValue("No group");
    expect(screen.getByRole("option", { name: "home-lab/others" })).toBeInTheDocument();
    expect(screen.getByLabelText("Port (optional)")).toHaveValue(null);
    await waitFor(() => expect(screen.queryByText("Loading authentication options…")).not.toBeInTheDocument());
    expect(screen.getByRole("radio", { name: "SSH private key" })).toBeChecked();
  });

  it("submits blank Port without launching anything and clears its dedicated password on success", async () => {
    const user = userEvent.setup();
    const harness = renderModal();
    await fillConnection(user);
    await user.click(await screen.findByRole("radio", { name: "Encrypted password for this connection" }));
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
    const harness = renderModal();
    await fillConnection(user);

    await user.click(screen.getByRole("radio", { name: "Saved password" }));
    expect(await screen.findByRole("option", { name: "office" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "id_work" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "New saved password" }));
    await user.type(screen.getByLabelText("Saved password name"), "lab-shared");
    await user.type(screen.getByLabelText("New password"), "shared-secret");
    await user.click(screen.getByRole("button", { name: "Create connection" }));
    await waitFor(() => expect(harness.createConnection).toHaveBeenCalledWith(expect.objectContaining({
      authentication: { kind: "new_shared_password", credential: "lab-shared", password: "shared-secret" },
    })));

    await user.click(screen.getByRole("radio", { name: "SSH private key" }));
    expect(screen.getByRole("option", { name: /id_work/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /id_work\.pub/ })).not.toBeInTheDocument();
  });

  it("can initialise or unlock the vault before creation", async () => {
    const user = userEvent.setup();
    const missing = renderModal({
      passwordVault: vi.fn().mockResolvedValue({ exists: false, unlocked: false, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 }),
    });
    await user.click(await screen.findByRole("radio", { name: "Encrypted password for this connection" }));
    await user.type(await screen.findByLabelText("Master password"), "a long master password");
    await user.type(screen.getByLabelText("Confirm master password"), "a long master password");
    await user.click(screen.getByRole("button", { name: "Create encrypted vault" }));
    await waitFor(() => expect(missing.initialiseVault).toHaveBeenCalledWith("a long master password"));

    missing.unmount();
    const locked = renderModal({
      passwordVault: vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 }),
    });
    await user.click(await screen.findByRole("radio", { name: "Encrypted password for this connection" }));
    await user.type(await screen.findByLabelText("Master password"), "the master password");
    await user.click(screen.getByRole("button", { name: "Unlock vault" }));
    await waitFor(() => expect(locked.unlockVault).toHaveBeenCalledWith("the master password"));
  });

  it("allows private-key creation while the password vault is locked", async () => {
    const user = userEvent.setup();
    const harness = renderModal({
      passwordVault: vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 }),
    });
    await fillConnection(user);
    await user.click(await screen.findByRole("radio", { name: "SSH private key" }));
    await user.click(screen.getByRole("button", { name: "Create connection" }));

    await waitFor(() => expect(harness.createConnection).toHaveBeenCalledWith(expect.objectContaining({
      authentication: { kind: "identity_file", keyId: privateKey.id },
    })));
    expect(harness.unlockVault).not.toHaveBeenCalled();
  });

  it("shows inline validation and a server problem without retaining the submitted secret", async () => {
    const user = userEvent.setup();
    const createConnection = vi.fn().mockRejectedValue(new Error("rejected"));
    renderModal({ createConnection });
    await user.click(await screen.findByRole("radio", { name: "Encrypted password for this connection" }));
    await user.type(screen.getByLabelText("Connection name"), "bad name");
    await user.tab();
    expect(screen.getByText("Use letters, numbers, dot, dash, or underscore; start with a letter or number.")).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Connection name"));
    await user.type(screen.getByLabelText("Connection name"), "good-name");
    await user.type(screen.getByLabelText("Host name or IP address"), "host.example");
    await user.type(screen.getByLabelText("Connection password"), "must-clear");
    await user.click(screen.getByRole("button", { name: "Create connection" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The connection could not be created");
    expect(screen.getByLabelText("Connection password")).toHaveValue("");
  });

  it("clears secrets on Cancel and Escape", async () => {
    const user = userEvent.setup();
    const cancel = renderModal();
    await user.click(await screen.findByRole("radio", { name: "Encrypted password for this connection" }));
    await user.type(await screen.findByLabelText("Connection password"), "cancel-secret");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(cancel.onClose).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText("Connection password")).toHaveValue("");

    await user.type(screen.getByLabelText("Connection password"), "escape-secret");
    await user.keyboard("{Escape}");
    expect(cancel.onClose).toHaveBeenCalledTimes(2);
    expect(screen.getByLabelText("Connection password")).toHaveValue("");
  });

  it("opens prerequisite screens with a resumable draft but never copies passwords into it", async () => {
    const user = userEvent.setup();
    const noKeys: KeyInventoryResponse = { ...inventory, items: [] };
    const harness = renderModal({
      groups: [],
      inventory: vi.fn().mockResolvedValue(noKeys),
    });

    await user.type(screen.getByLabelText("Connection name"), "lab-node");
    await user.type(screen.getByLabelText("Host name or IP address"), "host.example");
    await user.type(await screen.findByLabelText("Connection password"), "must-not-leave-modal");
    await user.click(screen.getByRole("button", { name: "Manage groups" }));

    expect(harness.onOpenPrerequisite).toHaveBeenCalledWith("Groups", expect.objectContaining({
      alias: "lab-node",
      hostName: "host.example",
      authentication: "dedicated_password",
    }));
    expect(JSON.stringify(harness.onOpenPrerequisite.mock.calls[0])).not.toContain("must-not-leave-modal");

    await user.click(screen.getByRole("button", { name: "Create a private key" }));
    expect(harness.onOpenPrerequisite).toHaveBeenCalledWith("Keys", expect.any(Object));
  });

  it("restores a safe draft and explains why creation is disabled", async () => {
    renderModal({
      initialDraft: {
        alias: "lab-node",
        group: "home-lab/others",
        hostName: "host.example",
        user: "root",
        port: "2202",
        authentication: "dedicated_password",
        savedCredential: "office",
        newCredential: "",
        keyID: "",
      },
    });

    expect(screen.getByLabelText("Connection name")).toHaveValue("lab-node");
    expect(screen.getByLabelText("Save in group")).toHaveValue("home-lab/others");
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("host.example");
    expect(screen.getByLabelText("User (optional)")).toHaveValue("root");
    expect(screen.getByLabelText("Port (optional)")).toHaveValue(2202);
    expect(await screen.findByText("Enter a connection password to continue.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create connection" })).toBeDisabled();
  });
});

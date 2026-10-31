import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useCallback, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { vaultStatus } from "../testing/vaultStatus";
import type { InspectorContent } from "../ui/Inspector";
import { KeysScreen } from "./KeysScreen";
import type { KeyInventoryResponse, KeysApi } from "./api";
import type { KeySecretsApi } from "./secretsApi";
import type { AboutApi } from "../api/about";

function buildSecrets(overrides: Partial<KeySecretsApi> = {}): KeySecretsApi {
  const listed = {
    credentials: [
      { kind: "key_passphrase", name: "build-key", uses: [] },
      { kind: "password", name: "office-vm", uses: ["web-1"] },
    ],
  };
  return {
    passwordVault: vi.fn().mockResolvedValue(vaultStatus()),
    credentials: vi.fn().mockResolvedValue(listed),
    storeCredential: vi.fn().mockResolvedValue(listed),
    assignCredential: vi.fn().mockResolvedValue(listed),
    unassignCredential: vi.fn().mockResolvedValue(listed),
    ...overrides,
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

// openTrash opens the trash from the list of groups, where its entries are listed in place of the keys.
async function openTrash() {
  await userEvent.click(await screen.findByRole("button", { name: /^Trash, \d+$/ }));
}

// renderInspector draws the details the screen last handed to the inspector, as the app's pane would.
function renderInspector(onInspector: ReturnType<typeof vi.fn>) {
  const content = onInspector.mock.calls.at(-1)?.[0] as InspectorContent;
  if (content === null || content === undefined) throw new Error("no details were handed to the inspector");
  return render(<>{content.body}</>);
}

// openKeyActions opens the row's "…" menu and returns it.
async function openKeyActions(row: HTMLElement): Promise<HTMLElement> {
  await userEvent.click(within(row).getByRole("button", { name: /^Actions for / }));
  return screen.getByRole("menu");
}

// The form to create a key opens from the heading's button.
async function openKeyCreation() {
  await userEvent.click(await screen.findByRole("button", { name: "Create a key" }));
}

async function openStoredPassphrase(row: HTMLElement) {
  const actions = await openKeyActions(row);
  await userEvent.click(within(actions).getByRole("menuitem", { name: "Save passphrase" }));
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
          ageDays: 40,
          stale: true,
          files: [
            {
              originalRelativePath: "id_old",
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
  it("opens key creation from the heading and offers safe next steps after in-process generation", async () => {
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

    const opener = await screen.findByRole("button", { name: "Create a key" });
    expect(opener).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByLabelText("File name")).not.toBeInTheDocument();
    await userEvent.click(opener);
    expect(opener).toHaveAttribute("aria-expanded", "true");
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

    await userEvent.click(within(screen.getByRole("form", { name: "Create a key" })).getByRole("button", { name: "Close" }));
    expect(screen.queryByLabelText("File name")).not.toBeInTheDocument();
  });

  it("lists classified files with the marks that stop a scan", async () => {
    render(<KeysScreen api={buildApi()} />);

    const inventoryTable = await screen.findByRole("table", {
      name: "Files classified by content and permissions",
    });
    // A table where its container has room for the columns, one card per key where it has not; never a
    // table wider than the page that hides its last column.
    expect(inventoryTable).toHaveClass("block", "w-full", "@xl/keytable:table");
    expect(inventoryTable.className).not.toMatch(/min-w-/);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    expect(workRow).toHaveClass("grid", "@xl/keytable:table-row");
    expect(within(workRow).getByText("referenced by 1")).toBeInTheDocument();

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

  it("puts each key creation field's name above it on narrow screens and beside it on wide ones", async () => {
    render(<KeysScreen api={buildApi()} />);
    await openKeyCreation();

    const fileName = await screen.findByLabelText("File name");
    const pair = fileName.closest("label");
    expect(pair).toHaveClass("grid", "lg:grid-cols-subgrid");
    expect(pair?.parentElement).toHaveClass("lg:grid-cols-[184px_minmax(0,1fr)]");
    expect(fileName).toHaveClass("min-h-10", "sm:min-h-0");
  });

  it("hands the chosen key to the inspector and opens it", async () => {
    const user = userEvent.setup();
    const onInspector = vi.fn();
    const onShowInspector = vi.fn();
    render(<KeysScreen api={buildApi()} onInspector={onInspector} onShowInspector={onShowInspector} />);

    await user.click(await screen.findByRole("button", { name: "id_work" }));
    expect(onInspector).toHaveBeenLastCalledWith(
      expect.objectContaining({ label: "Key details", attention: false }),
    );
    expect(onShowInspector).toHaveBeenCalledTimes(1);
    renderInspector(onInspector);
    expect(screen.getAllByText("SHA256:abcdef").length).toBeGreaterThan(0);
  });

  it("hands the chosen key to an inspector held in the parent's state only once", async () => {
    const user = userEvent.setup();
    const handed = vi.fn();
    // Stops a hand-over loop so a regression fails instead of hanging the run.
    const handOverLimit = 20;
    const api = buildApi();
    // Like App, the parent keeps the inspector in its own state behind a stable
    // setter, so every hand-over renders the parent and KeysScreen again.
    function InspectorHolder() {
      const [inspector, setInspector] = useState<InspectorContent>(null);
      const showInspector = useCallback((content: InspectorContent) => {
        handed(content);
        if (handed.mock.calls.length <= handOverLimit) setInspector(content);
      }, []);
      return (
        <>
          <KeysScreen api={api} onInspector={showInspector} />
          <output>{inspector?.label ?? ""}</output>
        </>
      );
    }
    render(<InspectorHolder />);

    await user.click(await screen.findByRole("button", { name: "id_work" }));
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Key details"));
    // A hand-over loop re-renders the parent and runs the effect again inside the
    // click's act, so the count is final once the details show; no wait is needed.
    const handedKeys = handed.mock.calls.filter(([content]) => content !== null);
    expect(handedKeys).toHaveLength(1);
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
    await openKeyCreation();
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
    const management = await openKeyActions(workRow);
    await userEvent.click(within(management).getByRole("menuitem", { name: "Change passphrase" }));
    // The form opens under the row it was asked from, not at the foot of the page.
    expect(screen.getByRole("heading", { name: "Change passphrase: id_work" }).closest("tr")).toHaveAttribute("data-key-panel-for", "key-one");

    await userEvent.type(screen.getByLabelText("Current passphrase"), "first passphrase");
    await userEvent.type(screen.getByLabelText("New passphrase"), "second passphrase");
    const passphraseForm = screen.getByRole("heading", { name: "Change passphrase: id_work" }).closest("form")!;
    await userEvent.click(within(passphraseForm).getByRole("button", { name: "Change passphrase" }));

    await waitFor(() =>
      expect(changePassphrase).toHaveBeenCalledWith("key-one", {
        currentPassphrase: "first passphrase",
        newPassphrase: "second passphrase",
        unencrypted: false,
      }),
    );
    await waitFor(() => expect(screen.queryByLabelText("Current passphrase")).not.toBeInTheDocument());

    const reopened = (await screen.findByRole("button", { name: "id_work" })).closest("tr")!;
    const reopenedActions = await openKeyActions(reopened);
    await userEvent.click(within(reopenedActions).getByRole("menuitem", { name: "Change passphrase" }));
    expect(screen.getByLabelText("Current passphrase")).toHaveValue("");
    expect(screen.getByLabelText("New passphrase")).toHaveValue("");
  });

  it("shows only the passphrase change as working while it waits, and sends it once", async () => {
    const user = userEvent.setup();
    let answer: () => void = () => undefined;
    const changePassphrase = vi.fn(() => new Promise((resolve) => {
      answer = () => resolve({ id: "key-one", relativePath: "id_work", encrypted: true, notes: [], transactionId: "tx" });
    }));
    render(<KeysScreen api={buildApi({ changePassphrase } as Partial<KeysApi>)} />);
    await openKeyCreation();

    const management = await openKeyActions(await screen.findByRole("row", { name: /id_work/ }));
    await user.click(within(management).getByRole("menuitem", { name: "Change passphrase" }));
    await user.type(screen.getByLabelText("Current passphrase"), "first passphrase");
    await user.type(screen.getByLabelText("New passphrase"), "second passphrase");
    const passphraseForm = screen.getByRole("heading", { name: "Change passphrase: id_work" }).closest("form")!;
    const submit = within(passphraseForm).getByRole("button", { name: "Change passphrase" });
    await user.click(submit);

    expect(submit).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Create key" })).not.toHaveAttribute("aria-busy");
    await user.type(screen.getByLabelText("New passphrase"), "{Enter}");
    expect(changePassphrase).toHaveBeenCalledTimes(1);
    answer();
    await waitFor(() => expect(screen.queryByLabelText("Current passphrase")).not.toBeInTheDocument());
  });

  it("shows key creation working until the key is made", async () => {
    const user = userEvent.setup();
    let answer: () => void = () => undefined;
    const generate = vi.fn(() => new Promise((resolve) => {
      answer = () => resolve({
        id: "key-new", relativePath: "id_new", publicRelativePath: "id_new.pub", fingerprint: "SHA256:new",
        keyType: "ssh-ed25519", bits: 256, encrypted: true, transactionId: "tx",
      });
    }));
    render(<KeysScreen api={buildApi({ generate } as Partial<KeysApi>)} />);

    await openKeyCreation();
    await user.type(await screen.findByLabelText("File name"), "id_new");
    await user.type(screen.getByLabelText("Passphrase"), "one-time secret");
    const create = screen.getByRole("button", { name: "Create key" });
    await user.click(create);

    expect(create).toHaveAttribute("aria-busy", "true");
    answer();
    expect(await screen.findByRole("button", { name: "Assign to a connection" })).toBeInTheDocument();
    expect(create).not.toHaveAttribute("aria-busy");
  });

  it("keeps a typed passphrase out of browser storage", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    render(<KeysScreen api={buildApi()} />);

    await screen.findByRole("row", { name: /id_work/ });
    await openKeyCreation();
    await userEvent.type(screen.getByLabelText("File name"), "id_new");
    await userEvent.type(screen.getByLabelText("Passphrase"), "correct horse");

    expect(setItem).not.toHaveBeenCalled();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it("counts the trash in the list of groups and lists it only when opened", async () => {
    render(<KeysScreen api={buildApi()} />);

    expect(await screen.findByRole("button", { name: "Trash, 1" })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /id_old/ })).not.toBeInTheDocument();
    await openTrash();
    expect(await screen.findByRole("row", { name: /id_old/ })).toBeInTheDocument();
  });

  it("requires a second confirmation before a permanent delete", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    expect(api.purge).not.toHaveBeenCalled();

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/cannot be undone/)).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm permanent deletion" }));
    await waitFor(() => expect(api.purge).toHaveBeenCalledWith("20260805T090000.000-aabbccdd"));
  });

  it("keeps the permanent delete confirmation open and says why when the delete fails", async () => {
    const api = buildApi({ purge: vi.fn().mockRejectedValue(new Error("api_mutation_failed")) });
    render(<KeysScreen api={api} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm permanent deletion" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The entry could not be deleted permanently.");
    expect(screen.getByRole("dialog")).toBe(dialog);
  });

  it("does not show the previous failure when the permanent delete confirmation is opened again after a cancel", async () => {
    const api = buildApi({ purge: vi.fn().mockRejectedValue(new Error("api_mutation_failed")) });
    render(<KeysScreen api={api} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    const failed = screen.getByRole("dialog");
    await userEvent.click(within(failed).getByRole("button", { name: "Confirm permanent deletion" }));
    await within(failed).findByRole("alert");
    await userEvent.click(within(failed).getByRole("button", { name: "Cancel" }));

    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    expect(within(screen.getByRole("dialog")).queryByRole("alert")).not.toBeInTheDocument();
  });

  it("leaves the trash entry intact when the second confirmation is cancelled", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));

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

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Restore" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The entry cannot be restored: id_old already exists");
    expect(screen.queryByText(/restore_path_occupied/)).not.toBeInTheDocument();
    expect(screen.getByRole("row", { name: /id_old/ })).toBeInTheDocument();
  });

  it("says why a trash entry cannot be restored in words instead of its code", async () => {
    const base = buildApi();
    const listed = await base.listTrash();
    const blocked = {
      ...listed,
      entries: listed.entries.map((entry) => ({
        ...entry,
        restorable: false,
        blockers: ["restore_fingerprint_present:id_new", "restore_entry_incomplete:id_old"],
      })),
    };
    render(<KeysScreen api={buildApi({ listTrash: vi.fn().mockResolvedValue(blocked) })} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    expect(trashRow).toHaveTextContent("the same key already exists as id_new, id_old is missing from the trash");
    expect(trashRow).not.toHaveTextContent("restore_");
  });

  it("marks a trash entry older than the retention window without deleting it", async () => {
    render(<KeysScreen api={buildApi()} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    expect(within(trashRow).getByText("40 days · older than 30 days")).toBeInTheDocument();
    expect(within(trashRow).getByRole("button", { name: "Restore" })).toBeEnabled();
  });

  it("reports an unreadable ssh directory instead of an empty list", async () => {
    const api = buildApi({ inventory: vi.fn().mockRejectedValue(new Error("api_read_failed")) });
    render(<KeysScreen api={api} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be read");
  });

  it("reads the ssh directory again when asked after the first read fails", async () => {
    const inventory = vi.fn()
      .mockRejectedValueOnce(new Error("api_read_failed"))
      .mockResolvedValue(buildInventory());
    render(<KeysScreen api={buildApi({ inventory })} />);

    await userEvent.click(await screen.findByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("row", { name: /id_work/ })).toBeInTheDocument();
  });

  it("keeps the list and says the list may be stale when the read after a successful operation fails", async () => {
    const inventory = vi.fn()
      .mockResolvedValueOnce(buildInventory())
      .mockRejectedValue(new Error("api_read_failed"));
    const api = buildApi({ inventory });
    render(<KeysScreen api={api} />);

    await openTrash();
    const trashRow = await screen.findByRole("row", { name: /id_old/ });
    await userEvent.click(within(trashRow).getByRole("button", { name: "Delete permanently" }));
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Confirm permanent deletion" }));

    expect(await screen.findByText(/The key list could not be read again/)).toBeInTheDocument();
    expect(api.purge).toHaveBeenCalled();
    expect(screen.getByRole("row", { name: /id_old/ })).toBeInTheDocument();
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
    const onInspector = vi.fn();
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} onInspector={onInspector} />);

    const row = await screen.findByRole("row", { name: /id_work-cert\.pub/ });
    expect(within(row).getByText("expired 2020-01-01 00:00Z")).toBeInTheDocument();
    await userEvent.click(within(row).getByRole("button", { name: "id_work-cert.pub" }));
    renderInspector(onInspector);
    expect(screen.getByText("key id aida@dubguild")).toBeInTheDocument();
    expect(screen.getByText("for deploy, ops")).toBeInTheDocument();
    expect(screen.getByText("signs ssh-ed25519 SHA256:signedkey")).toBeInTheDocument();
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
    const onInspector = vi.fn();
    render(<KeysScreen api={buildApi({ inventory: vi.fn().mockResolvedValue(inventory) })} onInspector={onInspector} />);

    const row = await screen.findByRole("row", { name: /host-cert\.pub/ });
    expect(within(row).getByText("never expires")).toBeInTheDocument();
    await userEvent.click(within(row).getByRole("button", { name: "host-cert.pub" }));
    renderInspector(onInspector);
    expect(screen.getByText("for any principal")).toBeInTheDocument();
  });

  it("refuses agent registration, and says what is missing, when no agent is reachable", async () => {
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    expect(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" })).toBeDisabled();
    expect(screen.getByText(/This process cannot connect to ssh-agent/)).toBeInTheDocument();
    expect(api.registerWithAgent).not.toHaveBeenCalled();
  });

  it("says how to give sshc an agent only for the system its engine runs on", async () => {
    const onWindows = { about: vi.fn().mockResolvedValue({ os: "windows" }) } as unknown as AboutApi;
    const { unmount } = render(<KeysScreen api={buildApi()} about={onWindows} />);
    expect(await screen.findByText(/Start the OpenSSH Authentication Agent service\./u)).toBeInTheDocument();
    expect(screen.queryByText(/SSH_AUTH_SOCK/u)).toBeNull();
    unmount();

    const onLinux = { about: vi.fn().mockResolvedValue({ os: "linux" }) } as unknown as AboutApi;
    render(<KeysScreen api={buildApi()} about={onLinux} />);
    expect(await screen.findByText(/SSH_AUTH_SOCK environment variable/u)).toBeInTheDocument();
    expect(screen.queryByText(/OpenSSH Authentication Agent/u)).toBeNull();
  });

  it("registers a key with the agent and the lifetime the user chose", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));

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
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));
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
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));

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
    const actions = await openKeyActions(row);
    expect(within(actions).queryByRole("menuitem", { name: "Show private key" })).not.toBeInTheDocument();
    await user.click(within(actions).getByRole("menuitem", { name: "Show public key" }));

    const shown = await screen.findByLabelText("Public key");
    expect(shown).toHaveTextContent("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmU aida@laptop");
    const detailRow = shown.closest("tr");
    expect(detailRow).toHaveAttribute("data-key-panel-for", "key-three");
    expect(detailRow?.closest("table")).toBe(row.closest("table"));
    expect(row.compareDocumentPosition(detailRow!) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    await user.click(screen.getByRole("button", { name: "Copy public key" }));

    expect(await navigator.clipboard.readText()).toBe(
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmU aida@laptop",
    );
  });

  it("opens private-key confirmation in the shared modal layer", async () => {
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
    const actions = await openKeyActions(row);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Show private key" }));

    const dialog = screen.getByRole("dialog");
    expect(dialog.closest("tr")).toBeNull();
    expect(document.body.contains(dialog)).toBe(true);
    expect(dialog).toHaveAttribute("aria-modal", "true");
    await userEvent.click(within(dialog).getByRole("button", { name: "Show private key" }));
    expect(await within(dialog).findByLabelText("Private key")).toHaveTextContent("BEGIN OPENSSH PRIVATE KEY");
  });

  it("offers no public key read for a private key", async () => {
    render(<KeysScreen api={buildApi()} />);

    const row = await screen.findByRole("row", { name: /id_work\b/ });
    const actions = await openKeyActions(row);
    expect(within(actions).queryByRole("menuitem", { name: "Show public key" })).not.toBeInTheDocument();
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
    const actions = await openKeyActions(row);
    await user.click(within(actions).getByRole("menuitem", { name: "Show public key" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("public key could not be read");
    expect(screen.queryByLabelText("Public key")).not.toBeInTheDocument();
  });

  it("keeps a key's public file in its row's details and menu, while a public-only key keeps its own row", async () => {
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
    const onInspector = vi.fn();
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventory) });
    render(<KeysScreen api={api} onInspector={onInspector} />);

    const privateRow = (await screen.findByRole("button", { name: "id_work" })).closest("tr")!;
    expect(screen.queryByRole("button", { name: "id_work.pub" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "colleague.pub" })).toBeVisible();

    const actions = await openKeyActions(privateRow);
    await user.click(within(actions).getByRole("menuitem", { name: "Show public key" }));
    await waitFor(() => expect(api.publicKey).toHaveBeenCalledWith("paired-public"));
    expect((await screen.findByLabelText("Public key")).closest("tr")).toHaveAttribute("data-key-panel-for", privateKey.id);

    await user.click(within(privateRow).getByRole("button", { name: "id_work" }));
    const details = renderInspector(onInspector);
    expect(within(details.container).getByText("id_work.pub")).toBeInTheDocument();
  });

  it("relocates a key and reports what moved and what it rewrote", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<KeysScreen api={api} />);

    const row = await screen.findByRole("row", { name: /id_work\b/ });
    const actions = await openKeyActions(row);
    await user.click(within(actions).getByRole("menuitem", { name: "Rename or move" }));

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
    const actions = await openKeyActions(row);
    await user.click(within(actions).getByRole("menuitem", { name: "Rename or move" }));
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
    // A search lists the public half as a row of its own.
    await userEvent.type(screen.getByRole("searchbox", { name: "Search keys" }), "id_work.pub");
    const publicRow = await screen.findByRole("row", { name: /id_work\.pub/ });
    const publicActions = await openKeyActions(publicRow);
    expect(within(publicActions).queryByRole("menuitem", { name: "Rename or move" })).not.toBeInTheDocument();
    const privateActions = await openKeyActions(privateRow);
    expect(within(privateActions).getByRole("menuitem", { name: "Rename or move" })).toBeInTheDocument();
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
    const actions = await openKeyActions(row);
    await user.click(within(actions).getByRole("menuitem", { name: "Rename or move" }));

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
    const actions = await openKeyActions(privateRow);
    await user.click(within(actions).getByRole("menuitem", { name: "Move to trash" }));

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
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ dedicatedKeyPassphrases: ["id_work"] })),
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
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ dedicatedKeyPassphrases: ["id_work"] })),
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
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ dedicatedKeyPassphrases: ["id_work"] })),
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
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Remove from ssh-agent" }));

    await waitFor(() => expect(api.deregisterFromAgent).toHaveBeenCalledWith("key-one"));
  });

  it("takes down the earlier failure once removing the key from the agent succeeds", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithLoadedKey()) });
    vi.mocked(api.deregisterFromAgent).mockRejectedValueOnce(new Error("api_mutation_failed"));
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Remove from ssh-agent" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("could not be removed from ssh-agent");

    const again = await openKeyActions(workRow);
    await userEvent.click(within(again).getByRole("menuitem", { name: "Remove from ssh-agent" }));
    await waitFor(() => expect(api.deregisterFromAgent).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
  });

  it("offers nothing to remove when the agent is not holding this key", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    render(<KeysScreen api={api} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    expect(within(actions).queryByRole("menuitem", { name: "Remove from ssh-agent" })).not.toBeInTheDocument();
  });

  it("offers a stored passphrase for the key, and never an account password", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    render(<KeysScreen api={api} secrets={buildSecrets()} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));

    const picker = await screen.findByLabelText("Use a stored passphrase");
    expect(picker).toHaveTextContent("build-key");
    expect(picker).not.toHaveTextContent("office-vm");
  });

  it("points a key at a stored passphrase, so adding it is one action", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    const secrets = buildSecrets();
    render(<KeysScreen api={api} secrets={secrets} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));
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
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));

    expect(await screen.findByText(/uses the stored passphrase named build-key/)).toBeInTheDocument();
    expect(screen.getByLabelText("Key passphrase")).toBeEnabled();
  });

  it("uses a key-dedicated saved passphrase for agent registration without exposing it", async () => {
    const api = buildApi({ inventory: vi.fn().mockResolvedValue(inventoryWithAgent()) });
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ dedicatedKeyPassphrases: ["id_work"] })),
    });
    render(<KeysScreen api={api} secrets={secrets} />);

    const workRow = await screen.findByRole("row", { name: /id_work/ });
    const actions = await openKeyActions(workRow);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Add to ssh-agent" }));

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

    fireEvent.dragStart(await screen.findByRole("row", { name: /id_work/ }), {
      dataTransfer: { setData: vi.fn(), effectAllowed: "" },
    });
    fireEvent.drop(screen.getByRole("button", { name: "archive, 0" }));

    await waitFor(() => expect(relocate).toHaveBeenCalledWith("key-one", { group: "archive" }));
  });

  it("opening one form from a row closes the one that was open", async () => {
    const secrets = buildSecrets({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus()),
    });
    render(<KeysScreen api={buildApi()} secrets={secrets} />);

    const row = await screen.findByRole("row", { name: /id_work/ });
    await openStoredPassphrase(row);
    expect(await screen.findByRole("heading", { name: /Saved passphrase/ })).toBeInTheDocument();

    const actions = await openKeyActions(row);
    await userEvent.click(within(actions).getByRole("menuitem", { name: "Change passphrase" }));

    expect(await screen.findByLabelText("Current passphrase")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Saved passphrase/ })).toBeNull();
  });
});

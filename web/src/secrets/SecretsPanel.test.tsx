import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { vaultStatus } from "../testing/vaultStatus";
import { SecretsPanel } from "./SecretsPanel";
import { ApiError } from "../api/client";
import type { SecretsApi } from "./SecretsPanel";

function buildApi(overrides: Partial<SecretsApi> = {}): SecretsApi {
  return {
    passwordVault: vi.fn().mockResolvedValue(vaultStatus()),
    initialiseVault: vi.fn().mockResolvedValue(vaultStatus()),
    unlockVault: vi.fn().mockResolvedValue(vaultStatus()),
    lockVault: vi.fn().mockResolvedValue(vaultStatus({ unlocked: false })),
    credentials: vi.fn().mockResolvedValue({
      credentials: [
        { kind: "password", name: "office-vm", uses: ["web-1", "web-2"], hosts: ["web-1", "web-2"] },
        {
          kind: "key_passphrase",
          name: "build-key",
          uses: ["keys/work/id_work", "keys/work/id_release"],
          hosts: ["build-1", "release-1"],
        },
        {
          kind: "totp",
          name: "production-otp",
          uses: ["bastion"],
          hosts: ["bastion"],
        },
      ],
      dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy-1"] }],
      keyHostUsageComplete: true,
    }),
    storeCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    revealCredential: vi.fn().mockImplementation(async (kind: string, name: string) => ({
      kind,
      name,
      secret: kind === "password" ? "original-password" : "original-passphrase",
    })),
    updateCredential: vi.fn().mockResolvedValue({
      credentials: [], dedicatedKeyPassphrases: [], keyHostUsageComplete: true,
    }),
    deleteCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    assignCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    unassignCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    totpCodes: vi.fn().mockResolvedValue({
      previous: "111111",
      current: "222222",
      next: "333333",
      periodSeconds: 30,
      remainingSeconds: 17,
    }),
    changeMasterPassword: vi.fn().mockResolvedValue({
      vault: vaultStatus(),
    }),
    updateStatus: vi.fn().mockResolvedValue({ current: "dev", available: false, restartRequired: false }),
    ...overrides,
  } as unknown as SecretsApi;
}

// openRowMenu opens a row's "…" menu and returns the menu, whose items are what the row can do.
async function openRowMenu(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(await screen.findByRole("button", { name: `Actions for ${name}` }));
  return screen.getByRole("menu");
}

describe("SecretsPanel", () => {
  it("names the page as Menu does and lists only its own kind, each with where it is used", async () => {
    render(<SecretsPanel api={buildApi()} kind="password" />);

    const list = await screen.findByRole("region", { name: "Account passwords" });
    expect(screen.getByRole("heading", { name: "Account passwords", level: 2 })).toBeInTheDocument();
    const office = within(list).getByRole("article", { name: "office-vm" });
    expect(within(office).getAllByText("web-1, web-2")[0]).toBeInTheDocument();
    expect(within(list).queryByText("build-key")).not.toBeInTheDocument();
    expect(within(list).queryByText("production-otp")).not.toBeInTheDocument();
  });

  it("names a key passphrase's keys and hosts, and a passphrase kept for one key", async () => {
    render(<SecretsPanel api={buildApi()} kind="key_passphrase" />);

    const build = await screen.findByRole("article", { name: "build-key" });
    expect(within(build).getAllByText("Keys: keys/work/id_work, keys/work/id_release; Hosts: build-1, release-1")[0]).toBeInTheDocument();
    const dedicated = screen.getByRole("article", { name: "keys/id_owned" });
    expect(within(dedicated).getByRole("heading", { name: "id_owned" })).toBeInTheDocument();
    expect(within(dedicated).getAllByText("Dedicated to this key; Hosts: deploy-1")[0]).toBeInTheDocument();
  });

  it("says a credential is not assigned anywhere", async () => {
    const api = buildApi({
      credentials: vi.fn().mockResolvedValue({
        credentials: [{ kind: "password", name: "unused-password", uses: [], hosts: [] }],
        dedicatedKeyPassphrases: [],
        keyHostUsageComplete: true,
      }),
    });
    render(<SecretsPanel api={api} kind="password" />);

    const password = await screen.findByRole("article", { name: "unused-password" });
    expect(within(password).getAllByText("Not assigned")[0]).toBeInTheDocument();
  });

  it("says when a key passphrase's hosts could not all be read, on its own page only", async () => {
    const credentials = vi.fn().mockResolvedValue({
      credentials: [
        { kind: "password", name: "office", uses: ["web-1"], hosts: ["web-1"] },
        { kind: "key_passphrase", name: "team", uses: ["keys/id_team"], hosts: [] },
      ],
      dedicatedKeyPassphrases: [],
      keyHostUsageComplete: false,
    });
    const { unmount } = render(<SecretsPanel api={buildApi({ credentials })} kind="key_passphrase" />);

    const team = await screen.findByRole("article", { name: "team" });
    expect(within(team).getAllByText("Keys: keys/id_team; Could not confirm assigned hosts")[0]).toBeInTheDocument();
    expect(screen.getByText(/could not be fully confirmed/i)).toBeInTheDocument();
    unmount();

    render(<SecretsPanel api={buildApi({ credentials })} kind="password" />);
    const office = await screen.findByRole("article", { name: "office" });
    expect(within(office).getAllByText("web-1")[0]).toBeInTheDocument();
    expect(screen.queryByText(/could not be fully confirmed/i)).not.toBeInTheDocument();
  });

  it("opens the form above the list from Add and stores the credential under its name", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} kind="password" />);

    await user.click(await screen.findByRole("button", { name: "Add" }));
    const form = screen.getByRole("region", { name: "Add an account password" });
    await user.type(within(form).getByLabelText("Name"), "office-vm");
    await user.type(within(form).getByLabelText("Password"), "s3cret");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.storeCredential).toHaveBeenCalledWith("password", "office-vm", "s3cret"));
    await waitFor(() => expect(screen.queryByRole("region", { name: "Add an account password" })).not.toBeInTheDocument());
  });

  it("keeps what was typed in the form when storing fails", async () => {
    const user = userEvent.setup();
    const api = buildApi({ storeCredential: vi.fn().mockRejectedValue(new Error("invalid TOTP")) });
    render(<SecretsPanel api={api} kind="totp" />);

    await user.click(await screen.findByRole("button", { name: "Add" }));
    const form = screen.getByRole("region", { name: "Add a one-time password" });
    await user.type(within(form).getByLabelText("Name"), "github");
    await user.type(within(form).getByLabelText("Base32 setup key or otpauth URI", { exact: true }), "not-base32");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await screen.findByText("The credential could not be saved.")).toBeInTheDocument();
    expect(within(form).getByLabelText("Name")).toHaveValue("github");
    expect(within(form).getByLabelText("Base32 setup key or otpauth URI", { exact: true })).toHaveValue("not-base32");
  });

  it("keeps the add form and a row's edit form open together, each with what was typed", async () => {
    const user = userEvent.setup();
    render(<SecretsPanel api={buildApi()} kind="password" />);

    await user.click(await screen.findByRole("button", { name: "Add" }));
    const addForm = screen.getByRole("region", { name: "Add an account password" });
    await user.type(within(addForm).getByLabelText("Name"), "half-typed");

    const menu = await openRowMenu(user, "office-vm");
    await user.click(within(menu).getByRole("menuitem", { name: "Edit" }));
    const editForm = await screen.findByRole("region", { name: "Edit account password" });
    await user.type(await within(editForm).findByLabelText("Name"), "-renamed");

    await user.click(screen.getByRole("button", { name: "Add" }));

    expect(within(screen.getByRole("region", { name: "Add an account password" })).getByLabelText("Name")).toHaveValue("half-typed");
    expect(within(screen.getByRole("region", { name: "Edit account password" })).getByLabelText("Name")).toHaveValue("office-vm-renamed");
  });

  it("offers the codes around the current one only once they have come", async () => {
    const user = userEvent.setup();
    render(<SecretsPanel api={buildApi({ totpCodes: vi.fn().mockReturnValue(new Promise(() => {})) })} kind="totp" />);

    const menu = await openRowMenu(user, "production-otp");

    expect(within(menu).getByRole("menuitem", { name: "Show the codes before and after" })).toBeDisabled();
  });

  it("shows a one-time password's current code and tells how to assign it", async () => {
    render(<SecretsPanel api={buildApi()} kind="totp" />);

    const token = await screen.findByRole("article", { name: "production-otp" });
    expect(await within(token).findByText("222 222")).toBeVisible();
    expect(within(token).queryByText("111 111")).not.toBeInTheDocument();
    expect(screen.getByText(/choose it under One-time password \(TOTP\) in Basic settings/)).toBeInTheDocument();
  });

  it("opens the codes around the current one under the row from its menu", async () => {
    const user = userEvent.setup();
    render(<SecretsPanel api={buildApi()} kind="totp" />);
    await screen.findByText("222 222");

    const menu = await openRowMenu(user, "production-otp");
    await user.click(within(menu).getByRole("menuitem", { name: "Show the codes before and after" }));

    expect(screen.getByText("111 111")).toBeVisible();
    expect(screen.getByText("333 333")).toBeVisible();
  });

  it("takes a one-time password off a host from the row's assigned hosts", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} kind="totp" />);

    const menu = await openRowMenu(user, "production-otp");
    await user.click(within(menu).getByRole("menuitem", { name: "Assigned hosts (1)" }));
    await user.click(screen.getByRole("button", { name: "Remove the one-time password assignment from bastion" }));

    await waitFor(() => expect(api.unassignCredential).toHaveBeenCalledWith("totp", "bastion"));
  });

  it("edits the saved account password under its row, name and value together", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} kind="password" />);

    const menu = await openRowMenu(user, "office-vm");
    await user.click(within(menu).getByRole("menuitem", { name: "Edit" }));

    const form = await screen.findByRole("region", { name: "Edit account password" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const name = await within(form).findByLabelText("Name");
    const value = within(form).getByLabelText("Password");
    expect(name).toHaveValue("office-vm");
    expect(value).toHaveValue("original-password");
    expect(value).toHaveAttribute("type", "password");

    await user.clear(name);
    await user.type(name, "office-shared");
    await user.clear(value);
    await user.type(value, "rotated-password");
    await user.click(within(form).getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(api.updateCredential).toHaveBeenCalledWith(
      "password", "office-vm", "office-shared", "rotated-password",
    ));
    await waitFor(() => expect(screen.queryByRole("region", { name: "Edit account password" })).not.toBeInTheDocument());
  });

  it("uses the same edit form for a named key passphrase", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} kind="key_passphrase" />);

    const menu = await openRowMenu(user, "build-key");
    await user.click(within(menu).getByRole("menuitem", { name: "Edit" }));

    const form = await screen.findByRole("region", { name: "Edit key passphrase" });
    expect(await within(form).findByLabelText("Name")).toHaveValue("build-key");
    expect(within(form).getByLabelText("Key passphrase")).toHaveValue("original-passphrase");
    expect(api.revealCredential).toHaveBeenCalledWith("key_passphrase", "build-key");
  });

  it("removes a passphrase kept for one key through its key, after asking", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} kind="key_passphrase" />);

    const menu = await openRowMenu(user, "keys/id_owned");
    expect(within(menu).queryByRole("menuitem", { name: "Edit" })).not.toBeInTheDocument();
    await user.click(within(menu).getByRole("menuitem", { name: "Delete" }));
    const confirmation = await screen.findByRole("dialog", { name: "Delete id_owned?" });
    await user.click(within(confirmation).getByRole("button", { name: "Delete it" }));

    await waitFor(() => expect(api.unassignCredential).toHaveBeenCalledWith("key_passphrase", "keys/id_owned"));
  });

  it("keeps a credential whose deletion is cancelled", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} kind="password" />);

    const menu = await openRowMenu(user, "office-vm");
    await user.click(within(menu).getByRole("menuitem", { name: "Delete" }));
    const confirmation = await screen.findByRole("dialog", { name: "Delete office-vm?" });
    await user.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(api.deleteCredential).not.toHaveBeenCalled();
  });

  it("says what still uses a credential the server refused to delete", async () => {
    const user = userEvent.setup();
    const api = buildApi({
      deleteCredential: vi.fn().mockRejectedValue(new ApiError("credential_in_use", 409, null)),
    });
    render(<SecretsPanel api={api} kind="password" />);

    const menu = await openRowMenu(user, "office-vm");
    await user.click(within(menu).getByRole("menuitem", { name: "Delete" }));
    const confirmation = await screen.findByRole("dialog", { name: "Delete office-vm?" });
    await user.click(within(confirmation).getByRole("button", { name: "Delete it" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/still assigned/i);
  });

  it("offers to unlock rather than showing an empty list", async () => {
    const api = buildApi({
      passwordVault: vi.fn().mockResolvedValue(vaultStatus({ unlocked: false })),
      credentials: vi.fn(),
    });
    render(<SecretsPanel api={api} kind="password" />);

    expect(await screen.findByLabelText("Master password")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Account passwords" })).not.toBeInTheDocument();
    expect(api.credentials).not.toHaveBeenCalled();
  });
});

it("does not offer manual locking for a passwordless vault", async () => {
  const api = buildApi({ passwordVault: vi.fn().mockResolvedValue(vaultStatus({ passwordless: true })) });
  render(<SecretsPanel api={api} kind="password" />);
  await screen.findByText("office-vm");
  expect(screen.queryByRole("button", { name: "Lock sshc" })).not.toBeInTheDocument();
});

it("does not show a lock screen when protection was removed before the lock request", async () => {
  const api = buildApi({ lockVault: vi.fn().mockResolvedValue(vaultStatus({ passwordless: true })) });
  const onLock = vi.fn();
  render(<SecretsPanel api={api} kind="password" onLock={onLock} />);
  await userEvent.click(await screen.findByRole("button", { name: "Lock sshc" }));
  await waitFor(() => expect(api.lockVault).toHaveBeenCalled());
  expect(onLock).not.toHaveBeenCalled();
});

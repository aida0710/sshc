import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SecretsPanel } from "./SecretsPanel";
import { ApiError } from "../api/client";
import type { IntegrationsApi } from "../api/integrations";

function buildApi(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    passwordVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] }),
    initialiseVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] }),
    unlockVault: vi.fn().mockResolvedValue({ exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] }),
    lockVault: vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [] }),
    credentials: vi.fn().mockResolvedValue({
      credentials: [
        { kind: "password", name: "office-vm", uses: ["web-1", "web-2"], hosts: ["web-1", "web-2"] },
        {
          kind: "key_passphrase",
          name: "build-key",
          uses: ["keys/work/id_work", "keys/work/id_release"],
          hosts: ["build-1", "release-1"],
        },
      ],
      dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy-1"] }],
      keyHostUsageComplete: true,
    }),
    storeCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    deleteCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    assignCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    unassignCredential: vi.fn().mockResolvedValue({ credentials: [] }),
    changeMasterPassword: vi.fn().mockResolvedValue({
      vault: { exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] },
      snapshotResealed: true,
    }),
    updateStatus: vi.fn().mockResolvedValue({ current: "dev", available: false, restartRequired: false }),
    ...overrides,
  } as unknown as IntegrationsApi;
}

describe("SecretsPanel", () => {
  it("labels hosts and keys for named and dedicated secrets without mixing kinds", async () => {
    const api = buildApi();
    render(<SecretsPanel api={api} />);

    const passwords = await screen.findByRole("region", { name: "Account passwords" });
    const office = within(passwords).getByRole("article", { name: "office-vm" });
    expect(within(office).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("web-1");
    expect(within(office).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("web-2");

    const phrases = screen.getByRole("region", { name: "Key passphrases" });
    const build = within(phrases).getByRole("article", { name: "build-key" });
    expect(within(build).getByRole("list", { name: "Keys" })).toHaveTextContent("keys/work/id_work");
    expect(within(build).getByRole("list", { name: "Keys" })).toHaveTextContent("keys/work/id_release");
    expect(within(build).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("build-1");
    expect(within(build).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("release-1");

    const dedicated = within(phrases).getByRole("article", { name: "keys/id_owned" });
    expect(within(dedicated).getByText("Dedicated to this key")).toBeInTheDocument();
    expect(within(dedicated).getByRole("list", { name: "Keys" })).toHaveTextContent("keys/id_owned");
    expect(within(dedicated).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("deploy-1");

    expect(within(passwords).queryByText("build-key")).not.toBeInTheDocument();
    expect(within(phrases).queryByText("office-vm")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Master password" })).not.toBeInTheDocument();
    expect(api.changeMasterPassword).not.toHaveBeenCalled();
  });

  it("distinguishes confirmed empty assignments from unavailable key hosts", async () => {
    const api = buildApi({
      credentials: vi.fn().mockResolvedValue({
        credentials: [
          { kind: "password", name: "unused-password", uses: [], hosts: [] },
          { kind: "key_passphrase", name: "unused-phrase", uses: [], hosts: [] },
        ],
        dedicatedKeyPassphrases: [],
        keyHostUsageComplete: true,
      }),
    });
    render(<SecretsPanel api={api} />);

    const password = await screen.findByRole("article", { name: "unused-password" });
    expect(within(password).getByText("No assigned hosts")).toBeInTheDocument();
    const phrase = screen.getByRole("article", { name: "unused-phrase" });
    expect(within(phrase).getByText("No assigned keys")).toBeInTheDocument();
    expect(within(phrase).getByText("No assigned hosts")).toBeInTheDocument();
  });

  it("keeps password hosts visible when key-host projection is incomplete", async () => {
    const api = buildApi({
      credentials: vi.fn().mockResolvedValue({
        credentials: [
          { kind: "password", name: "office", uses: ["web-1"], hosts: ["web-1"] },
          { kind: "key_passphrase", name: "team", uses: ["keys/id_team"], hosts: [] },
        ],
        dedicatedKeyPassphrases: [],
        keyHostUsageComplete: false,
      }),
    });
    render(<SecretsPanel api={api} />);

    expect(await screen.findByRole("status")).toHaveTextContent(/could not be fully confirmed/i);
    const office = screen.getByRole("article", { name: "office" });
    expect(within(office).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("web-1");
    const team = screen.getByRole("article", { name: "team" });
    expect(within(team).getByText("Could not confirm assigned hosts")).toBeInTheDocument();
    expect(within(team).queryByText("No assigned hosts")).not.toBeInTheDocument();
  });

  it("removes a dedicated passphrase through its key subject", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} />);

    const dedicated = await screen.findByRole("article", { name: "keys/id_owned" });
    await user.click(within(dedicated).getByRole("button", { name: "Remove saved passphrase for keys/id_owned" }));

    await waitFor(() =>
      expect(api.unassignCredential).toHaveBeenCalledWith("key_passphrase", "keys/id_owned"),
    );
  });

  it("creates an account password under a name", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} />);
    await screen.findByRole("region", { name: "Account passwords" });

    await user.type(screen.getByLabelText("New account password name"), "the office VMs");
    await user.type(screen.getByLabelText("New account password value"), "s3cret");
    await user.click(screen.getByRole("button", { name: "Store account password" }));

    await waitFor(() =>
      expect(api.storeCredential).toHaveBeenCalledWith("password", "the office VMs", "s3cret"),
    );
  });

  it("creates a key passphrase under the other kind", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SecretsPanel api={api} />);
    await screen.findByRole("region", { name: "Key passphrases" });

    await user.type(screen.getByLabelText("New key passphrase name"), "build");
    await user.type(screen.getByLabelText("New key passphrase value"), "phrase");
    await user.click(screen.getByRole("button", { name: "Store key passphrase" }));

    await waitFor(() =>
      expect(api.storeCredential).toHaveBeenCalledWith("key_passphrase", "build", "phrase"),
    );
  });

  // 2 台のマシンがまだ指している名前を削除すれば、後でどこか別の
  // 場所で両方が壊れる。サーバーは拒否し、画面は何が拒否したかを伝える。
  it("says what still uses a credential the server refused to delete", async () => {
    const user = userEvent.setup();
    const api = buildApi({
      deleteCredential: vi.fn().mockRejectedValue(new ApiError("credential_in_use", 409, null)),
    });
    render(<SecretsPanel api={api} />);
    const passwords = await screen.findByRole("region", { name: "Account passwords" });

    await user.click(within(passwords).getByRole("button", { name: "Delete office-vm" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/still uses/i);
  });

  // 起動時には何も尋ねられない。閉じた vault はここでそう言い、開くことを
  // 申し出る。「シークレットが消えた」と読める空リストを見せるのではなく。
  it("offers to unlock rather than showing an empty list", async () => {
    const api = buildApi({
      passwordVault: vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [] }),
      credentials: vi.fn(),
    });
    render(<SecretsPanel api={api} />);

    expect(await screen.findByLabelText("Master password")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Account passwords" })).not.toBeInTheDocument();
    expect(api.credentials).not.toHaveBeenCalled();
  });

});

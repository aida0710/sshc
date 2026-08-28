import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RemoteKeyPanel } from "./RemoteKeyPanel";
import { ApiError } from "../api/client";
import type { RemoteKeyPlan, RemoteKeysApi } from "./api";
import type { KeysApi } from "../keys/api";

afterEach(() => {
  vi.restoreAllMocks();
});

const publicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS deploy@laptop";
const fingerprint = "SHA256:bytFrSjxj2qRszG8sHhWN+YO3b9vDSU3gQtMorwKpEs";

const plan: RemoteKeyPlan = {
  alias: "bastion",
  user: "deploy",
  hostname: "bastion.example.com",
  port: "22",
  valuesFrom: "engine",
  fingerprint,
  keyPath: "~/.ssh/id_ed25519.pub",
  keyLine: publicKey,
  remotePath: "~/.ssh/authorized_keys",
  routine: "set -e\numask 077\ngrep -qxF \"$key\" \"$HOME/.ssh/authorized_keys\"\n",
  supported: true,
  manual: [
    "Open a session to the host yourself and check which shell the account uses.",
    "Append the public key line shown above to ~/.ssh/authorized_keys as a single line.",
  ],
  executableDirectives: [],
  actionToken: "a".repeat(43),
  actionExpiresAt: "2026-08-05T09:02:00Z",
};

const proxyCommand = {
  keyword: "ProxyCommand",
  command: "/usr/bin/nc %h %p",
  path: "~/.ssh/config",
  line: 4,
  onEvaluate: false,
  onConnect: true,
  overridable: false,
};

function buildKeys(overrides: Partial<Pick<KeysApi, "inventory" | "publicKey">> = {}) {
  return {
    inventory: vi.fn().mockResolvedValue({
      items: [
        {
          id: "key-pub",
          relativePath: "id_ed25519.pub",
          kind: "public_key",
          container: "",
          algorithm: "ed25519",
          keyType: "ssh-ed25519",
          bits: 256,
          encrypted: false,
          fingerprint,
          comment: "aida@laptop",
          permission: "0644",
          permissionRisk: false,
          sizeBytes: 100,
          references: [],
          notes: [],
        },
        {
          id: "key-private",
          relativePath: "id_ed25519",
          kind: "private_key",
          container: "OPENSSH PRIVATE KEY",
          algorithm: "ed25519",
          keyType: "ssh-ed25519",
          bits: 256,
          encrypted: true,
          fingerprint,
          comment: "",
          permission: "0600",
          permissionRisk: false,
          sizeBytes: 400,
          references: [],
          notes: [],
        },
      ],
      unreadable: [],
      agentDelegations: [],
      unresolvedReferences: [],
      agentAvailable: false,
      agentIdentities: [],
    }),
    publicKey: vi.fn().mockResolvedValue({
      id: "key-pub",
      relativePath: "id_ed25519.pub",
      publicKey: `${publicKey}\n`,
      fingerprint,
      comment: "aida@laptop",
    }),
    ...overrides,
  };
}

function buildApi(overrides: Partial<RemoteKeysApi> = {}): RemoteKeysApi {
  return {
    plan: vi.fn().mockResolvedValue(plan),
    register: vi.fn().mockResolvedValue({ outcome: "added", exitCode: 0, stderr: "", truncated: false }),
    ...overrides,
  };
}

async function fillForm() {
  await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
  await userEvent.type(screen.getByLabelText("Public key file"), "~/.ssh/id_ed25519.pub");
  await userEvent.type(screen.getByLabelText("Public key line"), publicKey);
}

async function fetchPlan(api: RemoteKeysApi) {
  render(<RemoteKeyPanel api={api} keys={buildKeys()} />);
  await fillForm();
  await userEvent.click(screen.getByRole("button", { name: "Show what this would do" }));
  return screen.findByRole("region", { name: "Confirm remote registration" });
}

describe("RemoteKeyPanel", () => {
  it("filters saved connections and registers the key on multiple selected hosts", async () => {
    const user = userEvent.setup();
    const api = buildApi({
      plan: vi.fn().mockImplementation(async (input) => ({
        ...plan,
        alias: input.alias,
        hostname: `${input.alias}.example.com`,
        actionToken: input.alias.padEnd(43, "x").slice(0, 43),
      })),
    });
    render(
      <RemoteKeyPanel
        api={api}
        keys={buildKeys()}
        hosts={["zeta", "db-prod", "alpha", "db-stage"]}
      />,
    );

    const search = screen.getByRole("searchbox", { name: "Host alias" });
    await user.type(search, "db");
    expect(screen.getByRole("checkbox", { name: "db-prod" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "db-stage" })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "alpha" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Select matches" }));
    expect(screen.getByText("2 selected")).toBeInTheDocument();
    await user.clear(search);
    expect(screen.getByRole("checkbox", { name: "db-prod" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "db-stage" })).toBeChecked();

    await user.type(screen.getByLabelText("Public key file"), "~/.ssh/id_ed25519.pub");
    await user.type(screen.getByLabelText("Public key line"), publicKey);
    await user.click(screen.getByRole("button", { name: "Show what this would do" }));

    await waitFor(() => expect(api.plan).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("region", { name: "Confirm registration on 2 hosts" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Register on 2 hosts" }));
    await waitFor(() => expect(api.register).toHaveBeenCalledTimes(2));
    expect(api.register).toHaveBeenCalledWith(expect.objectContaining({ alias: "db-prod" }));
    expect(api.register).toHaveBeenCalledWith(expect.objectContaining({ alias: "db-stage" }));
    expect(screen.getByText("db-prod", { selector: "li span" })).toBeInTheDocument();
    expect(screen.getByText("db-stage", { selector: "li span" })).toBeInTheDocument();
  });

  it("preloads a handed-off public key from fresh inventory without planning or registering", async () => {
    const api = buildApi();
    const keys = buildKeys();
    const onPreferredPublicKeyHandled = vi.fn();
    render(
      <RemoteKeyPanel
        api={api}
        keys={keys}
        preferredPublicKeyPath="id_ed25519.pub"
        onPreferredPublicKeyHandled={onPreferredPublicKeyHandled}
      />,
    );

    await waitFor(() => expect(screen.getByLabelText("Public key file")).toHaveValue("id_ed25519.pub"));
    expect(screen.getByLabelText("Public key line")).toHaveValue(publicKey);
    expect(keys.inventory).toHaveBeenCalled();
    expect(keys.publicKey).toHaveBeenCalledWith("key-pub");
    expect(onPreferredPublicKeyHandled).toHaveBeenCalledOnce();
    expect(api.plan).not.toHaveBeenCalled();
    expect(api.register).not.toHaveBeenCalled();
  });

  it("does not let a delayed handed-off key replace inputs that were already planned", async () => {
    const manualKey = publicKey.replace("deploy@laptop", "manual@laptop");
    let resolvePublicKey!: (value: Awaited<ReturnType<KeysApi["publicKey"]>>) => void;
    const pendingPublicKey = new Promise<Awaited<ReturnType<KeysApi["publicKey"]>>>((resolve) => {
      resolvePublicKey = resolve;
    });
    const keys = buildKeys({ publicKey: vi.fn().mockReturnValue(pendingPublicKey) });
    const api = buildApi();
    const onPreferredPublicKeyHandled = vi.fn();
    const user = userEvent.setup();
    render(
      <RemoteKeyPanel
        api={api}
        keys={keys}
        preferredPublicKeyPath="id_ed25519.pub"
        onPreferredPublicKeyHandled={onPreferredPublicKeyHandled}
      />,
    );
    await waitFor(() => expect(keys.publicKey).toHaveBeenCalledWith("key-pub"));

    await user.type(screen.getByLabelText("Host alias"), "bastion");
    await user.type(screen.getByLabelText("Public key file"), "~/.ssh/id_manual.pub");
    await user.type(screen.getByLabelText("Public key line"), manualKey);
    await user.click(screen.getByRole("button", { name: "Show what this would do" }));
    await screen.findByRole("region", { name: "Confirm remote registration" });

    resolvePublicKey({
      id: "key-pub",
      relativePath: "id_ed25519.pub",
      publicKey: `${publicKey}\n`,
      fingerprint,
      comment: "aida@laptop",
    });
    await waitFor(() => expect(screen.getByLabelText("Public key file")).toHaveValue("~/.ssh/id_manual.pub"));
    expect(screen.getByLabelText("Public key line")).toHaveValue(manualKey);
    expect(onPreferredPublicKeyHandled).toHaveBeenCalledOnce();

    await user.click(screen.getByRole("button", { name: "Register the key" }));
    await waitFor(() => expect(api.register).toHaveBeenCalledWith({
      alias: "bastion",
      keyPath: "~/.ssh/id_manual.pub",
      publicKey: manualKey,
      acknowledgeExecutable: false,
      actionToken: plan.actionToken,
    }));
  });

  it("names the task rather than the implementation area", () => {
    render(<RemoteKeyPanel api={buildApi()} keys={buildKeys()} />);

    expect(screen.getByRole("heading", { name: "Install Key on Server" })).toBeInTheDocument();
  });

  it("shows the alias, the effective user, the fingerprint and the change before any remote request", async () => {
    const api = buildApi();
    const confirmation = await fetchPlan(api);

    expect(within(confirmation).getByText("bastion")).toBeInTheDocument();
    expect(within(confirmation).getByText("deploy")).toBeInTheDocument();
    expect(within(confirmation).getByText("bastion.example.com:22")).toBeInTheDocument();
    expect(within(confirmation).getByText(fingerprint)).toBeInTheDocument();
    expect(within(confirmation).getByText(/Add one line to ~\/\.ssh\/authorized_keys/)).toBeInTheDocument();
    expect(within(confirmation).getByLabelText("Public key line to append")).toHaveTextContent(publicKey);
    expect(within(confirmation).getByLabelText("Remote command")).toHaveTextContent("umask 077");
    expect(api.register).not.toHaveBeenCalled();
  });

  it("keeps the confirm control unavailable until a plan has been fetched", async () => {
    const api = buildApi();
    render(<RemoteKeyPanel api={api} keys={buildKeys()} />);

    expect(screen.getByRole("button", { name: "Register the key" })).toBeDisabled();
    await fillForm();
    expect(screen.getByRole("button", { name: "Register the key" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "Show what this would do" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Register the key" })).toBeEnabled());

    await userEvent.click(screen.getByRole("button", { name: "Register the key" }));
    await waitFor(() =>
      expect(api.register).toHaveBeenCalledWith({
        alias: "bastion",
        keyPath: "~/.ssh/id_ed25519.pub",
        publicKey,
        acknowledgeExecutable: false,
        actionToken: plan.actionToken,
      }),
    );
    expect(await screen.findByText(/added/)).toBeInTheDocument();
  });

  it("withdraws the plan when the target changes, so the confirmation always matches", async () => {
    const api = buildApi();
    const confirmation = await fetchPlan(api);
    expect(confirmation).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Host alias"), "-two");

    expect(screen.queryByRole("region", { name: "Confirm remote registration" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Register the key" })).toBeDisabled();
    expect(api.register).not.toHaveBeenCalled();
  });

  it("offers manual instructions and no register control when the plan is unsupported", async () => {
    const api = buildApi({ plan: vi.fn().mockResolvedValue({ ...plan, supported: false }) });
    const confirmation = await fetchPlan(api);

    expect(within(confirmation).getByText(/Append the public key line shown above/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Register the key" })).not.toBeInTheDocument();
  });

  it("falls back to manual instructions when the remote turns out to be unsupported", async () => {
    const api = buildApi({
      register: vi.fn().mockRejectedValue(
        new ApiError("unsupported_remote", 422, {
          code: "unsupported_remote",
          message: "this remote environment does not provide the POSIX shell this operation needs",
        }),
      ),
    });
    await fetchPlan(api);

    await userEvent.click(screen.getByRole("button", { name: "Register the key" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("unsupported_remote");
    expect(await screen.findByText(/Append the public key line shown above/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Register the key" })).not.toBeInTheDocument();
  });

  it("will not register until an unavoidable executable directive is acknowledged", async () => {
    const api = buildApi({
      plan: vi.fn().mockResolvedValue({ ...plan, executableDirectives: [proxyCommand] }),
    });
    const confirmation = await fetchPlan(api);

    expect(within(confirmation).getByText("/usr/bin/nc %h %p")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Register the key" })).toBeDisabled();

    await userEvent.click(screen.getByLabelText(/accept that connecting runs it/));
    await userEvent.click(screen.getByRole("button", { name: "Register the key" }));

    await waitFor(() =>
      expect(api.register).toHaveBeenCalledWith({
        alias: "bastion",
        keyPath: "~/.ssh/id_ed25519.pub",
        publicKey,
        acknowledgeExecutable: true,
        actionToken: plan.actionToken,
      }),
    );
  });

  it("surfaces the refusal code from the plan endpoint and never offers a confirmation", async () => {
    const api = buildApi({
      plan: vi.fn().mockRejectedValue(
        new ApiError("invalid_public_key", 400, {
          code: "invalid_public_key",
          message: "public key must be exactly one valid OpenSSH public key line",
        }),
      ),
    });
    render(<RemoteKeyPanel api={api} keys={buildKeys()} />);
    await fillForm();
    await userEvent.click(screen.getByRole("button", { name: "Show what this would do" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("invalid_public_key");
    expect(screen.queryByRole("region", { name: "Confirm remote registration" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Register the key" })).toBeDisabled();
  });

  it("surfaces the refusal code from the register endpoint and stores nothing", async () => {
    const frameworkGlobals = ["IS_REACT_ACT_ENVIRONMENT"];
    const globalsBefore = Object.keys(window);
    const api = buildApi({
      register: vi.fn().mockRejectedValue(
        new ApiError("executable_directive_not_acknowledged", 409, {
          code: "executable_directive_not_acknowledged",
          message: "connecting would run a configured command that has not been acknowledged",
        }),
      ),
    });
    await fetchPlan(api);

    await userEvent.click(screen.getByRole("button", { name: "Register the key" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("executable_directive_not_acknowledged");
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    const added = Object.keys(window).filter(
      (key) => !globalsBefore.includes(key) && !frameworkGlobals.includes(key),
    );
    expect(added).toEqual([]);
  });

  it("adds no second status region to the shell", async () => {
    const api = buildApi();
    render(<RemoteKeyPanel api={api} keys={buildKeys()} />);

    expect(screen.queryAllByRole("status")).toHaveLength(0);
  });
  it("picks a public key from the inventory instead of asking for it to be typed", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    const keys = buildKeys();
    render(<RemoteKeyPanel api={api} keys={keys} />);

    const picker = await screen.findByLabelText("Public key from ~/.ssh");
    expect(within(picker).getByRole("option", { name: /id_ed25519\.pub/ })).toBeInTheDocument();
    expect(within(picker).queryByRole("option", { name: /id_ed25519 ·/ })).not.toBeInTheDocument();

    await user.selectOptions(picker, "key-pub");

    await waitFor(() => expect(screen.getByLabelText("Public key file")).toHaveValue("id_ed25519.pub"));
    expect(screen.getByLabelText("Public key line")).toHaveValue(publicKey);
    expect(keys.publicKey).toHaveBeenCalledWith("key-pub");
    expect(api.plan).not.toHaveBeenCalled();
  });

  it("withdraws a standing plan when a different key is picked", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    const confirmation = await fetchPlan(api);
    expect(confirmation).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Public key from ~/.ssh"), "key-pub");

    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "Confirm remote registration" })).not.toBeInTheDocument(),
    );
    expect(api.register).not.toHaveBeenCalled();
  });

  it("keeps the typed fields usable when the inventory cannot be read", async () => {
    const api = buildApi();
    const keys = buildKeys({ inventory: vi.fn().mockRejectedValue(new Error("api_read_failed")) });
    render(<RemoteKeyPanel api={api} keys={keys} />);

    await waitFor(() => expect(keys.inventory).toHaveBeenCalled());
    expect(screen.getByLabelText("Public key file")).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await fillForm();
    expect(screen.getByLabelText("Public key line")).toHaveValue(publicKey);
  });
});

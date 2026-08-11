import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConnectionsPage } from "./ConnectionsPage";
import { ApiError } from "../api/client";
import { configApi } from "../api/config";
import { dragMimeType, type DragPayload } from "./dragdrop";
import { integrationsApi } from "../api/integrations";
import { keysApi } from "../keys/api";

vi.mock("../api/config", async () => {
  const actual = await vi.importActual<typeof import("../api/config")>("../api/config");
  return {
    ...actual,
    configApi: {
      overview: vi.fn(), host: vi.fn(), file: vi.fn(), preview: vi.fn(), save: vi.fn(), renameGroup: vi.fn(),
      createConnection: vi.fn(), updateConnection: vi.fn(),
    },
  };
});

vi.mock("../api/integrations", () => ({
  integrationsApi: {
    terminalLaunch: vi.fn(), terminalOptions: vi.fn(), passwordVault: vi.fn(), credentials: vi.fn(),
    passwordEligibility: vi.fn(), initialiseVault: vi.fn(), unlockVault: vi.fn(),
  },
}));

vi.mock("../keys/api", async () => {
  const actual = await vi.importActual<typeof import("../keys/api")>("../keys/api");
  return { ...actual, keysApi: { inventory: vi.fn() } };
});

const overview = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [{ file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 }],
  hosts: [{
    identity: { path: "config", alias: "bastion" },
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    line: 1, patterns: ["bastion"], editable: true,
  }],
  metadata: { schemaVersion: 1 },
  groups: [],
  diagnostics: [],
  notices: [],
};

const detail = {
  form: {
    entry: overview.hosts[0],
    fields: [{ line: 2, keyword: "Port", values: ["22"], category: "basic", editable: true }],
    raw: "Host bastion\n\tPort 22\n",
  },
  metadata: { identity: { path: "config", alias: "bastion" } },
  effective: { alias: "bastion", approximate: true, entries: [] },
  file: {
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    contents: "Host bastion\n\tPort 22\n", digest: "digest", editable: true, exists: true,
  },
};

beforeEach(() => {
  // モジュールの factory はこれらを vi.fn()で作っており、restoreMocks は
  // それに手を触れないため、呼び出し記録はテストごとに手動でクリアする必要がある。
  vi.clearAllMocks();
  vi.mocked(configApi.overview).mockResolvedValue(overview as never);
  vi.mocked(configApi.host).mockResolvedValue(detail as never);
  vi.mocked(integrationsApi.terminalLaunch).mockResolvedValue({ command: "ssh bastion" } as never);
  vi.mocked(integrationsApi.terminalOptions).mockResolvedValue({
    selected: "terminal",
    terminals: [
      { id: "terminal", installed: true },
      { id: "iterm2", installed: true },
      { id: "kitty", installed: false },
      { id: "ghostty", installed: true },
      { id: "wezterm", installed: false },
      { id: "custom", installed: true },
    ],
    applications: [
      { name: "Term", path: "/Applications/Term.app" },
      { name: "Safari", path: "/Applications/Safari.app" },
    ],
  } as never);
  vi.mocked(integrationsApi.passwordVault).mockResolvedValue({
    exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12,
  } as never);
  vi.mocked(integrationsApi.credentials).mockResolvedValue({ credentials: [] } as never);
  vi.mocked(integrationsApi.passwordEligibility).mockResolvedValue({
    alias: "bastion", storable: true, blockers: [], warnings: [],
  } as never);
  vi.mocked(keysApi.inventory).mockResolvedValue({
    items: [], unreadable: [], agentDelegations: [], unresolvedReferences: [], agentAvailable: false, agentIdentities: [],
  } as never);
});

describe("ConnectionsPage", () => {
  it("replaces the bare section URL with the default server browser and discards old query state", async () => {
    const onNavigateLocation = vi.fn();
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{ pathname: "/connections", search: "?tab=raw" }}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await waitFor(() => expect(configApi.overview).toHaveBeenCalled());
    expect(onNavigateLocation).toHaveBeenCalledWith("/connections/servers", { replace: true });
    expect(configApi.host).not.toHaveBeenCalled();
  });

  it("opens a connection and tab from the URL", async () => {
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{
          pathname: "/connections/servers",
          search: "?path=config&host=bastion&panel=advanced&advanced=directives",
        }}
        onNavigateLocation={vi.fn()}
      />,
    );

    await waitFor(() => expect(configApi.host).toHaveBeenCalledWith("config", "bastion"));
    expect(await screen.findByRole("tab", { name: "Advanced settings" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Directives" })).toHaveAttribute("aria-selected", "true");
  });

  it("writes connection selection and tab changes to browser history", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{ pathname: "/connections/servers", search: "" }}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    expect(onNavigateLocation).toHaveBeenCalledWith(
      "/connections/servers?path=config&host=bastion&panel=basic",
    );

    await user.click(await screen.findByRole("tab", { name: "Settings analysis" }));
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=config&host=bastion&panel=analysis",
    );
  });

  it("changes group browser paths without refetching or losing the selected draft", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      hosts: [{ ...overview.hosts[0], group: "home" }],
      groups: [{
        name: "home",
        directory: "connections/home",
        keyDirectory: "keys/home",
        memberCount: 1,
        directoryPresent: true,
      }],
    } as never);
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{
          pathname: "/connections/servers",
          search: "?path=config&host=bastion&panel=basic",
        }}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await screen.findByRole("heading", { name: "bastion" });
    await user.clear(screen.getByLabelText("Port"));
    await user.type(screen.getByLabelText("Port"), "2222");
    expect(configApi.host).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Groups" }));
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/groups?path=config&host=bastion&panel=basic",
    );
    await user.click(screen.getByRole("button", { name: /^home,/ }));
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/groups/home?path=config&host=bastion&panel=basic",
    );
    expect(configApi.host).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText("Port")).toHaveValue(2222);
  });

  it("distinguishes an invalid connection URL from a missing declared group", async () => {
    const onNavigateLocation = vi.fn();
    const harness = render(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{ pathname: "/connections/files", search: "" }}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    expect(await screen.findByText("This connection URL is not recognised.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Back to servers" }));
    expect(onNavigateLocation).toHaveBeenCalledWith("/connections/servers", { replace: true });

    harness.rerender(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{ pathname: "/connections/groups/gone", search: "" }}
        onNavigateLocation={onNavigateLocation}
      />,
    );
    expect(await screen.findByText("Group not found.")).toBeInTheDocument();
    expect(screen.queryByText("This connection URL is not recognised.")).not.toBeInTheDocument();
  });

  it("shows a recovery action when a deep-linked connection no longer exists", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.host).mockRejectedValue(new Error("not found"));
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        location={{ pathname: "/connections/servers", search: "?path=config&host=gone&panel=basic" }}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "Back to connections" }));
    expect(onNavigateLocation).toHaveBeenCalledWith("/connections/servers", { replace: true });
  });

  it("keeps selection-specific notices out of the empty detail pane", async () => {
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      notices: [
        { code: "wildcard_shadow", path: "config", line: 9 },
        { code: "unnamed_host_block", path: "config", line: 9 },
        { code: "duplicate_alias", path: "config", line: 1 },
      ],
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);

    expect(await screen.findByText(/Another block declares the same alias/)).toBeInTheDocument();
    expect(screen.queryByText(/catch-all block can override/)).not.toBeInTheDocument();
    expect(screen.queryByText(/no concrete alias/)).not.toBeInTheDocument();
  });

  it("loads the tree, opens a host and saves a Basic field with the loaded base", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.updateConnection).mockResolvedValue({
      transactionId: "t1", written: ["config"], preview: { operation: "connection.update", diffs: [] },
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    const input = await screen.findByLabelText("Port");
    await user.clear(input);
    await user.type(input, "2222");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    await waitFor(() => expect(configApi.updateConnection).toHaveBeenCalledWith({
      identity: { path: "config", alias: "bastion" },
      base: "Host bastion\n\tPort 22\n",
      port: { action: "set", value: 2222 },
      password: { kind: "unchanged" },
      keyPassphrase: { kind: "unchanged" },
    }));
    expect(configApi.host).toHaveBeenCalledWith("config", "bastion");
  });

  it("shares one committed resource load between the summary and persistent Basic editor", async () => {
    const user = userEvent.setup();
    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    expect(await screen.findByText("bastion:22")).toBeInTheDocument();
    expect(keysApi.inventory).toHaveBeenCalledTimes(1);
    expect(integrationsApi.passwordVault).toHaveBeenCalledTimes(1);
    expect(integrationsApi.credentials).toHaveBeenCalledTimes(1);
    expect(integrationsApi.passwordEligibility).toHaveBeenCalledTimes(1);

    const port = screen.getByLabelText("Port");
    await user.clear(port);
    await user.type(port, "2222");
    expect(screen.getByText("bastion:22")).toBeInTheDocument();
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^bastion/ })).not.toHaveAttribute("draggable", "true");

    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    expect(screen.getByRole("region", { name: "Manage connection" })).toHaveAttribute("aria-disabled", "true");
    await user.click(screen.getByRole("tab", { name: "Settings analysis" }));
    await user.click(screen.getByRole("tab", { name: "Basic" }));
    expect(screen.getByLabelText("Port")).toHaveValue(2222);
  });

  it("registers a same-identity-aware navigation blocker and beforeunload guard for drafts", async () => {
    const user = userEvent.setup();
    let blocker: ((next: { pathname: string; search: string }) => boolean) | null = null;
    const onNavigationBlockerChange = vi.fn((next) => {
      blocker = next;
    });
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigationBlockerChange={onNavigationBlockerChange}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.clear(screen.getByLabelText("Port"));
    await user.type(screen.getByLabelText("Port"), "2222");
    await waitFor(() => expect(blocker).not.toBeNull());
    const activeBlocker = blocker as unknown as (next: { pathname: string; search: string }) => boolean;

    expect(activeBlocker({
      pathname: "/connections/groups/home",
      search: "?path=config&host=bastion&panel=analysis",
    })).toBe(true);
    expect(confirm).not.toHaveBeenCalled();
    expect(activeBlocker({ pathname: "/keys", search: "" })).toBe(false);
    expect(confirm).toHaveBeenCalledOnce();

    const beforeUnload = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(beforeUnload);
    expect(beforeUnload.defaultPrevented).toBe(true);

    confirm.mockReturnValue(true);
    expect(activeBlocker({ pathname: "/keys", search: "" })).toBe(true);
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    await waitFor(() => expect(onNavigationBlockerChange).toHaveBeenLastCalledWith(null));
  });

  it("keeps the selected connection and draft when a guarded host switch is rejected", async () => {
    const user = userEvent.setup();
    const edgeHost = {
      ...overview.hosts[0],
      identity: { path: "config", alias: "edge" },
      patterns: ["edge"],
      line: 4,
    };
    const edgeDetail = {
      ...detail,
      form: { ...detail.form, entry: edgeHost, raw: "Host edge\n\tPort 22\n" },
      metadata: { identity: edgeHost.identity },
      effective: { ...detail.effective, alias: "edge" },
      file: { ...detail.file, contents: "Host edge\n\tPort 22\n", digest: "edge" },
    };
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      hosts: [...overview.hosts, edgeHost],
    } as never);
    vi.mocked(configApi.host).mockImplementation(async (_path, alias) =>
      alias === "edge" ? edgeDetail as never : detail as never);
    let blocker: ((next: { pathname: string; search: string }) => boolean) | null = null;
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const onNavigateLocation = vi.fn((url: string) => {
      const next = new URL(url, "http://localhost");
      return blocker?.({ pathname: next.pathname, search: next.search }) ?? true;
    });
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
        onNavigationBlockerChange={(next) => {
          blocker = next;
        }}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /^bastion/ }));
    await user.clear(screen.getByLabelText("Port"));
    await user.type(screen.getByLabelText("Port"), "2222");
    await user.click(screen.getByRole("button", { name: /^edge/ }));

    expect(confirm).toHaveBeenCalledOnce();
    expect(configApi.host).not.toHaveBeenCalledWith("config", "edge");
    expect(screen.getByRole("heading", { name: "bastion" })).toBeInTheDocument();
    expect(screen.getByLabelText("Port")).toHaveValue(2222);

    confirm.mockReturnValue(true);
    await user.click(screen.getByRole("button", { name: /^edge/ }));
    expect(await screen.findByRole("heading", { name: "edge" })).toBeInTheDocument();
    expect(configApi.host).toHaveBeenCalledWith("config", "edge");
  });

  it("reports a post-commit reload failure as saved instead of inviting a duplicate save", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.updateConnection).mockResolvedValue({
      transactionId: "t1", written: ["config"], preview: { operation: "connection.update", diffs: [] },
    } as never);
    vi.mocked(configApi.host)
      .mockResolvedValueOnce(detail as never)
      .mockRejectedValueOnce(new Error("reload failed"));

    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    const input = await screen.findByLabelText("Port");
    await user.clear(input);
    await user.type(input, "2222");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(await screen.findByText(
      "The settings were saved, but the updated connection could not be loaded. Reload this connection.",
    )).toBeInTheDocument();
    expect(screen.queryByText(/Nothing was changed/)).not.toBeInTheDocument();
    expect(configApi.updateConnection).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Reload saved connection" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "Reload saved connection" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled());
  });

  it("opens the selected host in Terminal only after an explicit connect action", async () => {
    const user = userEvent.setup();
    render(<ConnectionsPage onInspector={() => undefined} />);

    expect(integrationsApi.terminalLaunch).not.toHaveBeenCalled();
    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    expect(integrationsApi.terminalLaunch).not.toHaveBeenCalled();
    await user.click(await screen.findByRole("button", { name: "Connect" }));

    expect(integrationsApi.terminalLaunch).toHaveBeenCalledWith("bastion");
  });

  it("keeps global terminal editing out of connection detail", async () => {
    const user = userEvent.setup();
    render(<ConnectionsPage onInspector={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: /bastion/ }));

    expect(screen.queryByLabelText("Open with")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Application")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Arguments")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled();
  });

  // サーバーが「選べる端末は一つも無い」と答えるプラットフォームでは、
  // Connect を無効にし、コマンドを自分で実行するよう伝える。
  it("keeps Connect unavailable when the server reports no terminal launcher", async () => {
    const user = userEvent.setup();
    vi.mocked(integrationsApi.terminalOptions).mockResolvedValue({
      selected: "terminal",
      terminals: [],
      applications: [],
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: /bastion/ }));

    expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
    expect(screen.getByText(/does not open a terminal for you/)).toBeInTheDocument();
  });

  // 「入っていない」と「開けなかった」は別の答えである。片方をもう片方の
  // 言葉で伝えると、直し方が消える。
  it("separates a terminal that is missing from a launch that failed", async () => {
    const user = userEvent.setup();
    vi.mocked(integrationsApi.terminalLaunch).mockRejectedValue(
      new ApiError("terminal_not_installed", 409, { code: "terminal_not_installed", message: "not installed" }),
    );
    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(await screen.findByRole("button", { name: "Connect" }));

    expect(await screen.findByText(/Terminal\.app was not found on this Mac/)).toBeInTheDocument();

    vi.mocked(integrationsApi.terminalLaunch).mockRejectedValue(
      new ApiError("terminal_launch_failed", 500, { code: "terminal_launch_failed", message: "refused" }),
    );
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(await screen.findByText(/Could not open bastion in Terminal/)).toBeInTheDocument();
  });

  it("keeps the diff of what was written on screen after the save reloads the host", async () => {
    // save は自分が書いたホストを再選択する。以前はこれが選択
    // effect に、同じ二つの値を持つ新しいオブジェクトを渡していたため、
    // effect が詳細を二度目に取得し、その答えが返るとプレビューを
    // 消していた。差分が見えていたのはちょうど一回のリクエストに
    // かかった時間だけだった。エンドツーエンドスイートはたまたまその
    // 窓の中を覗いていたため見えていたが、そうでなければ CI で失敗していた。
    const user = userEvent.setup();
    vi.mocked(configApi.updateConnection).mockResolvedValue({
      transactionId: "t1",
      written: ["config"],
      preview: {
        operation: "connection.update",
        diffs: [{
          path: "config",
          lines: [
            { op: "delete", text: "\tPort 22", oldLine: 2 },
            { op: "insert", text: "\tPort 2299", newLine: 2 },
          ],
        }],
      },
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    const input = await screen.findByLabelText("Port");
    await user.clear(input);
    await user.type(input, "2299");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    // 流れ全体で二回。一回は選択のため、もう一回は save 自身が
    // 行うリロードのため。三回目は差分を消してしまう重複である。
    await waitFor(() => expect(configApi.host).toHaveBeenCalledTimes(2));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(configApi.host).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("region", { name: "Save preview" })).toHaveTextContent("Port 2299");
  });

  it("discards the diff when a different connection is opened", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      hosts: [
        ...overview.hosts,
        {
          identity: { path: "config", alias: "nas" },
          file: { path: "config", absolute: "/home/tester/.ssh/config" },
          line: 5, patterns: ["nas"], editable: true,
        },
      ],
    } as never);
    vi.mocked(configApi.updateConnection).mockResolvedValue({
      transactionId: "t1",
      written: ["config"],
      preview: {
        operation: "connection.update",
        diffs: [{ path: "config", lines: [{ op: "insert", text: "\tPort 2299", newLine: 2 }] }],
      },
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    const input = await screen.findByLabelText("Port");
    await user.clear(input);
    await user.type(input, "2299");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));
    await screen.findByText(/Port 2299/);

    await user.click(screen.getByRole("button", { name: /nas/ }));

    // 差分が記述するのは、もはや開いていないブロックのバイトである。
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Save preview" }))
        .toHaveTextContent("Change a value to see exactly what would be written."));
  });

  it("hides the previous connection while a newly selected detail is still loading", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      hosts: [
        ...overview.hosts,
        {
          identity: { path: "config", alias: "nas" },
          file: { path: "config", absolute: "/home/tester/.ssh/config" },
          line: 5, patterns: ["nas"], editable: true,
        },
      ],
    } as never);
    vi.mocked(configApi.host).mockImplementation(async (_path, alias) => {
      if (alias === "bastion") return detail as never;
      return await new Promise(() => undefined);
    });

    render(<ConnectionsPage onInspector={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    expect(await screen.findByRole("heading", { name: /^bastion$/ })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /nas/ }));

    expect(screen.queryByRole("heading", { name: /^bastion$/ })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Choose a connection" })).toBeInTheDocument();
  });

  it("keeps pattern rules out of the server browser and never asks for their host detail", async () => {
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      hosts: [
        ...overview.hosts,
        {
          identity: { path: "", alias: "" },
          file: { path: "config", absolute: "/home/tester/.ssh/config" },
          line: 9, patterns: ["*"], wildcard: true, editable: true,
        },
      ],
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);

    expect(await screen.findByRole("button", { name: "bastion" })).toBeInTheDocument();
    expect(screen.queryByText("Host *")).not.toBeInTheDocument();
    expect(configApi.host).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "Choose a connection" })).toBeInTheDocument();
  });

  it("keeps the edit visible and shows the conflict when the file changed on disk", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.updateConnection).mockRejectedValue(new ApiError("config_conflict", 409, {
      code: "config_conflict",
      message: "request rejected",
      path: "config",
      conflict: {
        path: "config",
        externalChange: [{ op: "insert", text: "Host other", newLine: 3 }],
        localChange: [{ op: "delete", text: "\tPort 22", oldLine: 2 }],
      },
    }));

    render(<ConnectionsPage onInspector={() => undefined} />);

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    const input = await screen.findByLabelText("Port");
    await user.clear(input);
    await user.type(input, "2222");
    await user.click(screen.getByRole("button", { name: "Save Basic settings" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/changed outside this application/i);
    expect(screen.getByText("Changed on disk since you loaded it")).toBeInTheDocument();
    expect(screen.getByLabelText("Port")).toHaveValue(2222);
  });

  it("creates a complete connection, refreshes the tree, and opens its Basic detail without launching a terminal", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    const createdDetail = {
      ...detail,
      form: {
        ...detail.form,
        entry: { ...detail.form.entry, identity: { path: "config", alias: "build01" }, patterns: ["build01"] },
        raw: "Host build01\n\tHostName build.example.com\n\tPort 22\n",
      },
      metadata: { identity: { path: "config", alias: "build01" } },
      effective: { ...detail.effective, alias: "build01" },
    };
    vi.mocked(configApi.createConnection).mockResolvedValue({
      transactionId: "t-create",
      identity: { path: "config", alias: "build01" },
      preview: { operation: "connection.create", diffs: [] },
    } as never);
    vi.mocked(configApi.host).mockResolvedValue(createdDetail as never);

    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "New connection" }));
    await user.type(await screen.findByLabelText("Connection name"), "build01");
    await user.type(screen.getByLabelText("Host name or IP address"), "build.example.com");
    await user.type(await screen.findByLabelText("Connection password"), "connection-only");
    await user.click(screen.getByRole("button", { name: "Create connection" }));

    await waitFor(() => expect(configApi.createConnection).toHaveBeenCalledWith({
      alias: "build01",
      group: "",
      hostName: "build.example.com",
      authentication: { kind: "dedicated_password", password: "connection-only" },
    }));
    await waitFor(() => expect(configApi.host).toHaveBeenCalledWith("config", "build01"));
    expect(configApi.overview).toHaveBeenCalledTimes(2);
    expect(integrationsApi.terminalLaunch).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog", { name: "Create connection" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { name: "build01" })).toBeInTheDocument();
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=config&host=build01&panel=basic",
      { replace: true },
    );
  });

  it("replaces the URL after renaming the selected connection", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t-rename", written: ["config"], preview: { operation: "config.rename", diffs: [] },
    } as never);
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    await user.clear(await screen.findByLabelText("Rename alias"));
    await user.type(screen.getByLabelText("Rename alias"), "gateway");
    await user.click(screen.getByRole("button", { name: "Rename" }));

    await waitFor(() => expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=config&host=gateway&panel=basic",
      { replace: true },
    ));
  });

  it("replaces a renamed connection URL before refreshing its detail", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.host)
      .mockResolvedValueOnce(detail as never)
      .mockRejectedValueOnce(new Error("detail refresh failed"));
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t-rename", written: ["config"], preview: { operation: "config.rename", diffs: [] },
    } as never);
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    await user.clear(await screen.findByLabelText("Rename alias"));
    await user.type(screen.getByLabelText("Rename alias"), "gateway");
    await user.click(screen.getByRole("button", { name: "Rename" }));

    await waitFor(() => expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=config&host=gateway&panel=basic",
      { replace: true },
    ));
  });

  it("moves a host to another file with both loaded bases", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      files: [
        { file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 },
        { file: { path: "conf.d/10-home.conf", absolute: "/home/tester/.ssh/conf.d/10-home.conf" }, editable: true, loads: 1 },
      ],
    } as never);
    vi.mocked(configApi.file).mockResolvedValue({
      file: { path: "conf.d/10-home.conf", absolute: "/home/tester/.ssh/conf.d/10-home.conf" },
      contents: "Host nas\n\tUser aida\n", digest: "digest", editable: true, exists: true,
    } as never);
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1",
      written: ["config", "conf.d/10-home.conf"],
      preview: { operation: "config.move", diffs: [] },
    } as never);

    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    expect(screen.getByText(/Primary group changes where sshc organises the connection/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Change storage file" })).toBeDisabled();
    await user.selectOptions(await screen.findByLabelText("Storage file"), "conf.d/10-home.conf");
    await user.click(screen.getByRole("button", { name: "Change storage file" }));

    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith({
      kind: "move",
      path: "config",
      base: "Host bastion\n\tPort 22\n",
      alias: "bastion",
      destinationPath: "conf.d/10-home.conf",
      destinationBase: "Host nas\n\tUser aida\n",
    }));
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=conf.d%2F10-home.conf&host=bastion&panel=basic",
      { replace: true },
    );
  });

  it("keeps the original connection selected when moving it to another file fails", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.overview).mockResolvedValue({
      ...overview,
      files: [
        { file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 },
        { file: { path: "conf.d/10-home.conf", absolute: "/home/tester/.ssh/conf.d/10-home.conf" }, editable: true, loads: 1 },
      ],
    } as never);
    vi.mocked(configApi.file).mockResolvedValue({
      file: { path: "conf.d/10-home.conf", absolute: "/home/tester/.ssh/conf.d/10-home.conf" },
      contents: "Host nas\n", digest: "digest", editable: true, exists: true,
    } as never);
    vi.mocked(configApi.save).mockRejectedValue(new Error("move conflict"));
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    await user.selectOptions(await screen.findByLabelText("Storage file"), "conf.d/10-home.conf");
    await user.click(screen.getByRole("button", { name: "Change storage file" }));

    await screen.findByRole("alert");
    expect(screen.getByRole("heading", { name: "bastion" })).toBeInTheDocument();
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=config&host=bastion&panel=basic",
    );
  });

  it("deletes the selected host block without touching the rest of the file", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "t1", written: ["config"], preview: { operation: "config.file_raw", diffs: [] },
    } as never);

    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    await user.click(await screen.findByRole("button", { name: "Delete connection" }));
    await user.click(screen.getByRole("button", { name: "Confirm delete" }));

    await waitFor(() => expect(configApi.save).toHaveBeenCalledWith({
      kind: "file_raw",
      path: "config",
      base: "Host bastion\n\tPort 22\n",
      raw: "",
    }));
    expect(onNavigateLocation).toHaveBeenLastCalledWith("/connections/servers", { replace: true });
  });

  it("keeps the selected host and URL when deletion fails", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    vi.mocked(configApi.save).mockRejectedValue(new Error("delete conflict"));
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    await user.click(await screen.findByRole("button", { name: "Delete connection" }));
    await user.click(screen.getByRole("button", { name: "Confirm delete" }));

    await screen.findByRole("alert");
    expect(screen.getByRole("heading", { name: "bastion" })).toBeInTheDocument();
    expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/servers?path=config&host=bastion&panel=basic",
    );
  });
});

describe("taking a connection out of every group", () => {
  const grouped = {
    ...detail,
    form: { ...detail.form, entry: { ...detail.form.entry, group: "work" } },
  };

  it("sends it to the entry file, with that file's own bytes as the precondition", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.host).mockResolvedValue(grouped as never);
    vi.mocked(configApi.file).mockResolvedValue({
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      contents: "Host other\n", digest: "d", editable: true, exists: true,
    } as never);
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "tx", written: [], preview: { operation: "config.move", diffs: [] },
    } as never);

    render(<ConnectionsPage onInspector={() => undefined} />);
    await user.click(await screen.findByRole("button", { name: /bastion/ }));
    await user.click(screen.getByRole("button", { name: "More connection actions" }));
    await user.selectOptions(await screen.findByLabelText("Primary group"), "");
    await user.click(screen.getByRole("button", { name: "Move to this group" }));

    await waitFor(() =>
      expect(configApi.save).toHaveBeenCalledWith(
        expect.objectContaining({
          kind: "move",
          alias: "bastion",
          destinationPath: "config",
          destinationBase: "Host other\n",
        }),
      ),
    );
  });
});

describe("dropping in the tree", () => {
  // jsdom にはドラッグ実装がないため、transfer は tree が触れる二つの
  // ものを運ぶスタブである。
  function transfer(payload: DragPayload) {
    const store = new Map<string, string>([[dragMimeType, JSON.stringify(payload)]]);
    return {
      types: [...store.keys()],
      setData: (type: string, value: string) => void store.set(type, value),
      getData: (type: string) => store.get(type) ?? "",
      effectAllowed: "move",
      dropEffect: "move",
    };
  }

  const grouped = {
    ...overview,
    hosts: [{
      identity: { path: "connections/home/nas.conf", alias: "nas" },
      file: { path: "connections/home/nas.conf", absolute: "/home/tester/.ssh/connections/home/nas.conf" },
      line: 1, patterns: ["nas"], editable: true, group: "home",
    }],
    groups: [
      {
        name: "home", directory: "connections/home", keyDirectory: "keys/home",
        memberCount: 1, directoryPresent: true,
      },
      {
        name: "work", directory: "connections/work", keyDirectory: "keys/work",
        memberCount: 0, directoryPresent: true,
      },
      {
        name: "home/eu", parent: "home", directory: "connections/home/eu", keyDirectory: "keys/home/eu",
        memberCount: 0, directoryPresent: true,
      },
    ],
    metadata: { schemaVersion: 2, groups: [{ name: "home" }, { name: "work" }, { name: "home/eu" }] },
  };

  function drag(source: HTMLElement, target: HTMLElement, payload: DragPayload) {
    fireEvent.dragStart(source, { dataTransfer: transfer(payload) });
    fireEvent.dragOver(target, { dataTransfer: transfer(payload) });
    fireEvent.drop(target, { dataTransfer: transfer(payload) });
  }

  beforeEach(() => {
    vi.mocked(configApi.overview).mockResolvedValue(grouped as never);
    vi.mocked(configApi.file).mockResolvedValue({
      file: { path: "connections/home/nas.conf", absolute: "/x" },
      contents: "Host nas\n", digest: "d", editable: true, exists: true,
    } as never);
    vi.mocked(configApi.save).mockResolvedValue({
      transactionId: "tx", written: [], preview: { operation: "config.move", diffs: [] },
    } as never);
  });

  it("moves a direct connection into a visible child group", async () => {
    const user = userEvent.setup();
    render(<ConnectionsPage onInspector={() => undefined} />);
    await screen.findByRole("button", { name: /nas/ });
    await user.click(screen.getByRole("button", { name: "Groups" }));
    await user.click(within(screen.getByRole("list", { name: "Groups" })).getByRole("button", { name: /^home,/ }));
    const row = screen.getByRole("button", { name: /nas/ });

    drag(row, screen.getByRole("button", { name: /^eu,/ }), {
      kind: "connection", path: "connections/home/nas.conf", alias: "nas", group: "home",
    });

    await waitFor(() =>
      expect(configApi.save).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "move", alias: "nas", destinationGroup: "home/eu" }),
      ),
    );
  });

  it("replaces the selected connection URL after dragging it into a child group", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    const moved = {
      ...grouped,
      hosts: [{
        ...grouped.hosts[0],
        identity: { path: "connections/home/eu/nas.conf", alias: "nas" },
        file: {
          path: "connections/home/eu/nas.conf",
          absolute: "/home/tester/.ssh/connections/home/eu/nas.conf",
        },
        group: "home/eu",
      }],
    };
    vi.mocked(configApi.overview)
      .mockResolvedValueOnce(grouped as never)
      .mockResolvedValue(moved as never);

    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );
    const row = await screen.findByRole("button", { name: /nas/ });
    await user.click(row);
    await user.click(screen.getByRole("button", { name: "Groups" }));
    await user.click(within(screen.getByRole("list", { name: "Groups" })).getByRole("button", { name: /^home,/ }));

    drag(screen.getByRole("button", { name: /nas/ }), screen.getByRole("button", { name: /^eu,/ }), {
      kind: "connection", path: "connections/home/nas.conf", alias: "nas", group: "home",
    });

    await waitFor(() => expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/groups/home?path=connections%2Fhome%2Feu%2Fnas.conf&host=nas&panel=basic",
      { replace: true },
    ));
  });

  it("moves a connection out of every group by sending it to the entry file", async () => {
    const user = userEvent.setup();
    render(<ConnectionsPage onInspector={() => undefined} />);
    await screen.findByRole("button", { name: /nas/ });
    await user.click(screen.getByRole("button", { name: "Groups" }));
    await user.click(within(screen.getByRole("list", { name: "Groups" })).getByRole("button", { name: /^home,/ }));
    const row = screen.getByRole("button", { name: /nas/ });

    drag(row, within(screen.getByRole("navigation", { name: "Group path" })).getByRole("button", { name: "Groups" }), {
      kind: "connection", path: "connections/home/nas.conf", alias: "nas", group: "home",
    });

    await waitFor(() =>
      expect(configApi.save).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "move", alias: "nas", destinationPath: "config" }),
      ),
    );
  });

  it("nests a group by renaming it under its new parent", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.renameGroup).mockResolvedValue({
      transactionId: "tx", written: [], preview: { operation: "config.group_rename", diffs: [] },
    } as never);
    render(<ConnectionsPage onInspector={() => undefined} />);
    await screen.findByRole("button", { name: /nas/ });
    await user.click(screen.getByRole("button", { name: "Groups" }));
    const groups = screen.getByRole("list", { name: "Groups" });
    const source = within(groups).getByRole("button", { name: /^work,/ });

    drag(source, within(groups).getByRole("button", { name: /^home,/ }), { kind: "group", name: "work" });

    await waitFor(() => expect(configApi.renameGroup).toHaveBeenCalledWith("work", "home/work"));
  });

  it("replaces the selected connection URL when its group is dragged", async () => {
    const user = userEvent.setup();
    const onNavigateLocation = vi.fn();
    const renamed = {
      ...grouped,
      hosts: [{
        ...grouped.hosts[0],
        identity: { path: "connections/work/home/nas.conf", alias: "nas" },
        file: {
          path: "connections/work/home/nas.conf",
          absolute: "/home/tester/.ssh/connections/work/home/nas.conf",
        },
        group: "work/home",
      }],
      metadata: {
        schemaVersion: 2,
        groups: [{ name: "work" }, { name: "work/home" }, { name: "work/home/eu" }],
      },
      groups: [
        {
          name: "work", directory: "connections/work", keyDirectory: "keys/work",
          memberCount: 0, directoryPresent: true,
        },
        {
          name: "work/home", parent: "work", directory: "connections/work/home", keyDirectory: "keys/work/home",
          memberCount: 1, directoryPresent: true,
        },
        {
          name: "work/home/eu", parent: "work/home", directory: "connections/work/home/eu", keyDirectory: "keys/work/home/eu",
          memberCount: 0, directoryPresent: true,
        },
      ],
    };
    vi.mocked(configApi.overview)
      .mockResolvedValueOnce(grouped as never)
      .mockResolvedValue(renamed as never);
    vi.mocked(configApi.renameGroup).mockResolvedValue({
      transactionId: "tx", written: [], preview: { operation: "config.group_rename", diffs: [] },
    } as never);
    render(
      <ConnectionsPage
        onInspector={() => undefined}
        onNavigateLocation={onNavigateLocation}
      />,
    );
    await user.click(await screen.findByRole("button", { name: /nas/ }));
    await user.click(screen.getByRole("button", { name: "Groups" }));
    const groups = screen.getByRole("list", { name: "Groups" });

    drag(
      within(groups).getByRole("button", { name: /^home,/ }),
      within(groups).getByRole("button", { name: /^work,/ }),
      { kind: "group", name: "home" },
    );

    await waitFor(() => expect(onNavigateLocation).toHaveBeenLastCalledWith(
      "/connections/groups?path=connections%2Fwork%2Fhome%2Fnas.conf&host=nas&panel=basic",
      { replace: true },
    ));
  });

  it("takes a nested group back to the top level", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.renameGroup).mockResolvedValue({
      transactionId: "tx", written: [], preview: { operation: "config.group_rename", diffs: [] },
    } as never);
    render(<ConnectionsPage onInspector={() => undefined} />);
    await screen.findByRole("button", { name: /nas/ });
    await user.click(screen.getByRole("button", { name: "Groups" }));
    await user.click(within(screen.getByRole("list", { name: "Groups" })).getByRole("button", { name: /^home,/ }));
    const source = screen.getByRole("button", { name: /^eu,/ });

    drag(
      source,
      within(screen.getByRole("navigation", { name: "Group path" })).getByRole("button", { name: "Groups" }),
      { kind: "group", name: "home/eu" },
    );

    await waitFor(() => expect(configApi.renameGroup).toHaveBeenCalledWith("home/eu", "eu"));
  });
});

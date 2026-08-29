import { StrictMode, useEffect } from "react";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

const openVault = () =>
  Promise.resolve({ exists: true, unlocked: true, aliases: [] as string[], dedicatedKeyPassphrases: [], minPassphraseLength: 12 });
import { App, resolveOSC52, vaultStatePollIntervalMs } from "./App";
import type { InspectorContent } from "./ui/Inspector";
import { LanguageProvider } from "./i18n/context";
import { ThemeProvider } from "./theme/context";
import { ja } from "./i18n/messages";
import { ApiError, apiClient } from "./api/client";
import { announceVaultLocked } from "./secrets/vaultLockSignal";

type BroadcastListener = (event: MessageEvent<unknown>) => void;

class FakeBroadcastChannel {
  static channels: FakeBroadcastChannel[] = [];
  readonly listeners = new Set<BroadcastListener>();
  closed = false;

  constructor(readonly name: string) {
    FakeBroadcastChannel.channels.push(this);
  }

  postMessage(data: unknown) {
    for (const channel of FakeBroadcastChannel.channels) {
      if (channel !== this && channel.name === this.name && !channel.closed) {
        for (const listener of channel.listeners) listener(new MessageEvent("message", { data }));
      }
    }
  }

  addEventListener(_type: "message", listener: BroadcastListener) {
    this.listeners.add(listener);
  }

  removeEventListener(_type: "message", listener: BroadcastListener) {
    this.listeners.delete(listener);
  }

  close() {
    this.closed = true;
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((accept, decline) => {
    resolve = accept;
    reject = decline;
  });
  return { promise, resolve, reject };
}

vi.mock("./connections/ConnectionsPage", () => ({
  ConnectionsPage: ({
    onInspector,
    creationDraft,
    onCreationDraftChange,
    onNavigateForCreation,
    location,
    onNavigateLocation,
    onNavigationBlockerChange,
    preferredKey,
    onPreferredKeyApplied,
  }: {
    onInspector: (content: InspectorContent) => void;
    creationDraft?: { alias: string } | null;
    onCreationDraftChange?: (draft: Record<string, string> | null) => void;
    onNavigateForCreation?: (section: "Groups" | "Keys") => void;
    location?: { pathname: string; search: string };
    onNavigateLocation?: (url: string, options?: { replace?: boolean }) => void;
    onNavigationBlockerChange?: (
      blocker: ((next: { pathname: string; search: string }) => boolean) | null,
    ) => void;
    preferredKey?: { privateKeyId: string; privateRelativePath: string } | null;
    onPreferredKeyApplied?: () => void;
  }) => {
    useEffect(() => {
      if (location?.pathname === "/connections") {
        onNavigateLocation?.("/connections/servers", { replace: true });
      }
    }, [location?.pathname, onNavigateLocation]);
    return <div>
      connections panel
      <span>{`connection location ${location?.pathname ?? "missing"}${location?.search ?? ""}`}</span>
      <span>{`preferred connection key ${preferredKey?.privateRelativePath ?? "none"}`}</span>
      <button type="button" onClick={onPreferredKeyApplied}>consume connection key</button>
      {creationDraft === null || creationDraft === undefined ? null : <span>{`draft ${creationDraft.alias}`}</span>}
      <button type="button" onClick={() => onInspector({ label: "Display and classification", attention: true, body: <p>inspector body</p> })}>
        offer inspector
      </button>
      <button
        type="button"
        onClick={() => onNavigateLocation?.("/connections/servers?path=config&host=build01&panel=advanced&advanced=directives")}
      >
        open routed host
      </button>
      <button type="button" onClick={() => onNavigationBlockerChange?.(() => false)}>
        block connection navigation
      </button>
      <button type="button" onClick={() => onNavigationBlockerChange?.(null)}>
        allow connection navigation
      </button>
      <button
        type="button"
        onClick={() => {
          onCreationDraftChange?.({
            alias: "lab-node", group: "", hostName: "host.example", user: "", port: "",
            authentication: "dedicated_password", savedCredential: "", newCredential: "", keyID: "",
          });
          onNavigateForCreation?.("Keys");
        }}
      >
        open key prerequisite
      </button>
    </div>;
  },
}));
vi.mock("./explorer/ConfigExplorer", () => ({
  ConfigExplorer: ({ target }: { target?: { path: string; line: number } | null }) => (
    <div>{`config panel ${target === null || target === undefined ? "no target" : `${target.path}:${target.line}`}`}</div>
  ),
}));
vi.mock("./groups/GroupsPanel", () => ({ GroupsPanel: () => <div>groups panel</div> }));
vi.mock("./history/HistoryPanel", () => ({ HistoryPanel: () => <div>history panel</div> }));
vi.mock("./keys/KeysScreen", () => ({
  KeysScreen: ({
    onAssignGeneratedKey,
    onInstallGeneratedKey,
  }: {
    onAssignGeneratedKey?: (key: { privateKeyId: string; privateRelativePath: string }) => void;
    onInstallGeneratedKey?: (key: { publicRelativePath: string }) => void;
  }) => (
    <div>
      keys panel
      <button type="button" onClick={() => onAssignGeneratedKey?.({ privateKeyId: "key-new", privateRelativePath: "id_new" })}>
        hand key to connection
      </button>
      <button type="button" onClick={() => onInstallGeneratedKey?.({ publicRelativePath: "id_new.pub" })}>
        hand key to server
      </button>
    </div>
  ),
}));
vi.mock("./diagnostics/DiagnosticsPanel", () => ({ DiagnosticsPanel: () => <div>diagnostics panel</div> }));
vi.mock("./settings/SettingsPanel", () => ({ SettingsPanel: () => <div>settings panel</div> }));
vi.mock("./secrets/SecretsPanel", () => ({
  SecretsPanel: ({ onLock }: { onLock: () => void }) => (
    <button type="button" onClick={onLock}>lock fixture</button>
  ),
}));
vi.mock("./knownhosts/KnownHostsPanel", () => ({ KnownHostsPanel: () => <div>known hosts panel</div> }));
vi.mock("./remotekeys/RemoteKeyPanel", () => ({
  RemoteKeyPanel: ({
    preferredPublicKeyPath,
    onPreferredPublicKeyHandled,
  }: {
    preferredPublicKeyPath?: string | null;
    onPreferredPublicKeyHandled?: () => void;
  }) => (
    <div>
      {`remote keys panel ${preferredPublicKeyPath ?? "no key"}`}
      <button type="button" onClick={onPreferredPublicKeyHandled}>consume public key</button>
    </div>
  ),
}));
vi.mock("./secrets/LockScreen", () => ({
  LockScreen: ({ exists, onOpen }: {
    exists: boolean;
    onOpen: (status?: { migratedFromVersion?: number; migratedToVersion?: number }) => void;
  }) => (
    <div>
      <span>{exists ? "existing vault fixture" : "new vault fixture"}</span>
      <button type="button" onClick={() => onOpen()}>unlock fixture</button>
      <button type="button" onClick={() => onOpen({ migratedFromVersion: 4, migratedToVersion: 5 })}>
        migrate fixture
      </button>
    </div>
  ),
}));

const csrfToken = "c".repeat(43);

async function openFromMenu(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole("link", { name: "Menu" }));
  const menu = await screen.findByRole("region", { name: "Menu" });
  await user.click(within(menu).getByRole("link", { name: `Open ${label}` }));
}

afterEach(() => {
  apiClient.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  FakeBroadcastChannel.channels = [];
  window.history.replaceState(null, "", "/");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("App", () => {
  it("resolves OSC 52 with an SSH host override before the terminal default", () => {
    expect(resolveOSC52(undefined, true)).toBe(true);
    expect(resolveOSC52("deny", true)).toBe(false);
    expect(resolveOSC52("allow", false)).toBe(true);
  });
  it("unmounts the ready UI when vault-state polling observes a lock", async () => {
    let poll: (() => void) | null = null;
    const originalSetInterval = window.setInterval.bind(window);
    vi.spyOn(window, "setInterval").mockImplementation((handler, delay, ...args) => {
      if (delay === vaultStatePollIntervalMs && typeof handler === "function") {
        poll = handler as () => void;
      }
      return originalSetInterval(handler, delay, ...args) as unknown as ReturnType<typeof setInterval>;
    });
    const vault = vi.fn()
      .mockResolvedValueOnce({ exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 })
      .mockResolvedValueOnce({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 });

    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vault}
      />,
    );

    expect(await screen.findByRole("heading", { name: "sshc" })).toBeInTheDocument();
    await waitFor(() => expect(poll).not.toBeNull());
    await act(async () => {
      poll?.();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(await screen.findByText("existing vault fixture")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "sshc" })).toBeNull();
  });

  it("conceals the ready subtree on resume and coalesces overlapping focus checks", async () => {
    let poll: (() => void) | null = null;
    const originalSetInterval = window.setInterval.bind(window);
    vi.spyOn(window, "setInterval").mockImplementation((handler, delay, ...args) => {
      if (delay === vaultStatePollIntervalMs && typeof handler === "function") poll = handler as () => void;
      return originalSetInterval(handler, delay, ...args) as unknown as ReturnType<typeof setInterval>;
    });
    const resumed = deferred<Awaited<ReturnType<typeof openVault>>>();
    const vault = vi.fn()
      .mockImplementationOnce(openVault)
      .mockReturnValueOnce(resumed.promise);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vault}
      />,
    );
    expect(await screen.findByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    await waitFor(() => expect(poll).not.toBeNull());

    act(() => poll?.());
    expect(screen.getByRole("navigation", { name: "Primary" })).toBeInTheDocument();

    act(() => {
      window.dispatchEvent(new Event("focus"));
      document.dispatchEvent(new Event("visibilitychange"));
    });

    expect(vault).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("navigation", { name: "Primary" })).toBeNull();
    expect(screen.getByText(/Checking that the vault is still unlocked/)).toBeVisible();

    await act(async () => resumed.resolve(await openVault()));
    expect(await screen.findByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    expect(vault).toHaveBeenCalledTimes(2);
  });

  it("keeps protected UI hidden after a failed resume check until a retry succeeds", async () => {
    let poll: (() => void) | null = null;
    const originalSetInterval = window.setInterval.bind(window);
    vi.spyOn(window, "setInterval").mockImplementation((handler, delay, ...args) => {
      if (delay === vaultStatePollIntervalMs && typeof handler === "function") poll = handler as () => void;
      return originalSetInterval(handler, delay, ...args) as unknown as ReturnType<typeof setInterval>;
    });
    const resumed = deferred<Awaited<ReturnType<typeof openVault>>>();
    const vault = vi.fn()
      .mockImplementationOnce(openVault)
      .mockReturnValueOnce(resumed.promise)
      .mockImplementationOnce(openVault);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vault}
      />,
    );
    expect(await screen.findByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    await waitFor(() => expect(poll).not.toBeNull());

    act(() => {
      window.dispatchEvent(new Event("focus"));
    });
    await act(async () => resumed.reject(new Error("offline")));

    expect(screen.queryByRole("navigation", { name: "Primary" })).toBeNull();
    expect(screen.getByText(/Protected content remains hidden while sshc retries/)).toBeVisible();

    await act(async () => {
      poll?.();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(await screen.findByRole("navigation", { name: "Primary" })).toBeInTheDocument();
  });

  it("applies a broadcast lock, cleans up its observer, and ignores a late resume response", async () => {
    vi.stubGlobal("BroadcastChannel", FakeBroadcastChannel);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    const resumed = deferred<Awaited<ReturnType<typeof openVault>>>();
    const vault = vi.fn()
      .mockImplementationOnce(openVault)
      .mockReturnValueOnce(resumed.promise);
    const view = render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vault}
      />,
    );
    expect(await screen.findByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    const observer = FakeBroadcastChannel.channels[0];

    act(() => {
      window.dispatchEvent(new Event("focus"));
    });
    act(() => announceVaultLocked());
    expect(await screen.findByText("existing vault fixture")).toBeInTheDocument();

    await act(async () => resumed.resolve(await openVault()));
    expect(screen.getByText("existing vault fixture")).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Primary" })).toBeNull();

    view.unmount();
    expect(observer?.closed).toBe(true);
  });

  it("keeps the sidebar compact and moves grouped destinations into Menu", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await screen.findByRole("heading", { name: "sshc" });

    expect(within(screen.getByRole("list", { name: "Start" })).getAllByRole("link").map((link) => link.textContent)).toEqual([
      "Home",
      "Connections",
      "SFTP",
    ]);
    expect(screen.getByRole("link", { name: "Menu" })).toHaveAttribute("href", "/menu");
    expect(screen.getByText("Sessions", { exact: true })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Terminal" })).toBeNull();

    await user.click(screen.getByRole("link", { name: "Menu" }));
    const menu = await screen.findByRole("region", { name: "Menu" });

    for (const label of [
      "Config",
      "Groups",
      "Keys",
      "Known Hosts",
      "Install Key on Server",
      "Ad hoc checks",
      "Secrets",
      "Settings",
      "Sync",
      "History",
    ]) {
      expect(within(menu).getByRole("link", { name: `Open ${label}` })).toHaveAttribute("href");
    }
    const maintenance = within(menu).getByRole("region", { name: "Maintenance" });
    expect(within(maintenance).queryByRole("heading", { name: "Maintenance" })).not.toBeNull();
    const settings = within(maintenance).getByRole("link", { name: "Open Settings" });
    expect(settings).toHaveAttribute("href", "/settings");
    expect(settings.querySelector("use")).toHaveAttribute("href", "#icon-settings");
  });

  it("restores and changes the desktop navigation layout", async () => {
    window.localStorage.setItem("sshc.navigation.visible", "false");
    window.localStorage.setItem("sshc.navigation.width", "312");
    const user = userEvent.setup();
    const { container } = render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await screen.findByRole("heading", { name: "sshc" });
    const layout = container.querySelector<HTMLElement>("[data-desktop-navigation-visible]");
    expect(layout).toHaveAttribute("data-desktop-navigation-visible", "false");
    expect(layout?.style.getPropertyValue("--navigation-width")).toBe("312px");

    await user.click(screen.getByRole("button", { name: "Show navigation" }));
    expect(layout).toHaveAttribute("data-desktop-navigation-visible", "true");
    expect(window.localStorage.getItem("sshc.navigation.visible")).toBe("true");

    const resize = screen.getByRole("separator", { name: "Resize navigation" });
    resize.focus();
    await user.keyboard("{ArrowRight}");
    expect(layout?.style.getPropertyValue("--navigation-width")).toBe("320px");
    expect(window.localStorage.getItem("sshc.navigation.width")).toBe("320");
  });

  it("keeps the inspector open across a section change", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await user.click(await screen.findByRole("link", { name: "Connections" }));
    await screen.findByText("connections panel");

    expect(screen.queryByRole("button", { name: /details/i })).toBeNull();

    await user.click(screen.getByRole("button", { name: "offer inspector" }));
    await user.click(screen.getByRole("button", { name: "Show Display and classification Needs attention" }));

    expect(screen.getByRole("complementary", { name: "Display and classification" })).toHaveTextContent("inspector body");

    await openFromMenu(user, "Keys");
    await user.click(screen.getByRole("link", { name: "Connections" }));
    await user.click(screen.getByRole("button", { name: "offer inspector" }));

    expect(screen.getByRole("complementary", { name: "Display and classification" })).toBeInTheDocument();
  });

  it("Androidの戻る操作では画面遷移より先に一時UIを閉じる", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );
    await user.click(await screen.findByRole("link", { name: "Connections" }));
    await user.click(await screen.findByRole("button", { name: "offer inspector" }));
    await user.click(screen.getByRole("button", { name: "Show Display and classification Needs attention" }));
    expect(screen.getByRole("complementary", { name: "Display and classification" })).toBeInTheDocument();

    const back = new Event("sshc-android-back", { cancelable: true });
    act(() => { window.dispatchEvent(back); });
    expect(back.defaultPrevented).toBe(true);
    expect(screen.queryByRole("complementary", { name: "Display and classification" })).toBeNull();
  });

  it("offers the three appearances and remembers the chosen one", async () => {
    const user = userEvent.setup();
    render(
      <ThemeProvider>
        <App
          bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
          health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
          vault={openVault}
        />
      </ThemeProvider>,
    );

    const control = await screen.findByLabelText("Appearance");
    expect(control).toHaveValue("dark");

    await user.selectOptions(control, "dark");

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(window.localStorage.getItem("sshc.theme")).toBe("dark");
  });

  it("shows the starting status before session setup completes", () => {
    render(<App bootstrap={() => new Promise(() => undefined)} health={vi.fn()} vault={openVault} />);

    expect(screen.getByRole("status")).toHaveTextContent("Starting secure local session…");
  });

  it("shows the authenticated shell after bootstrap and health succeed", async () => {
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
      />,
    );

    expect(await screen.findByRole("heading", { name: "sshc" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Local session active · 0.1.0");
    expect(screen.getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
    for (const label of ["Home", "Connections", "SFTP", "Menu"]) {
      expect(screen.getByRole("link", { name: label })).toHaveAttribute("href");
    }
    expect(document.body).not.toHaveTextContent(csrfToken);
  });

  it("renders a direct section URL and links every primary destination", async () => {
    window.history.replaceState(null, "", "/keys");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByText("keys panel")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Menu" })).toHaveAttribute("href", "/menu");
    expect(screen.getByRole("link", { name: "Connections" })).toHaveAttribute("href", "/connections");
  });

  it("keeps the shell visible while a direct section module resolves without loading copy", async () => {
    window.history.replaceState(null, "", "/keys");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByRole("link", { name: "Menu" })).toHaveAttribute("href", "/menu");
    expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
    expect(await screen.findByText("keys panel")).toBeInTheDocument();
  });

  it("keeps an invalid connection sub-route inside the Connections section for its local recovery", async () => {
    window.history.replaceState(null, "", "/connections/removed-view");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByText("connection location /connections/removed-view"))
      .toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Connections" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("renders Settings directly and follows browser history back to it", async () => {
    window.history.replaceState(null, "", "/settings");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByText("settings panel")).toBeInTheDocument();
    expect(window.location.pathname).toBe("/settings");

    act(() => {
      window.history.pushState(null, "", "/history");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(await screen.findByText("history panel")).toBeInTheDocument();

    act(() => {
      window.history.replaceState(null, "", "/settings");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(screen.getByText("settings panel")).toBeInTheDocument();
  });

  it("treats Menu as a transient navigation hub in browser history", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/settings");
    const replaced = vi.spyOn(window.history, "replaceState");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByText("settings panel")).toBeInTheDocument();
    await user.click(screen.getByRole("link", { name: "Menu" }));
    await user.click(await screen.findByRole("link", { name: "Open History" }));
    expect(window.location.pathname).toBe("/history");
    expect(replaced).toHaveBeenLastCalledWith(null, "", "/history");
  });

  it("starts each section at the top of its own scroll surface", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/config");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    const config = await screen.findByText("config panel no target");
    const configScroller = config.parentElement;
    expect(configScroller).not.toBeNull();
    configScroller!.scrollTop = 320;

    await openFromMenu(user, "Keys");

    const keys = await screen.findByText("keys panel");
    const keysScroller = keys.parentElement;
    expect(keysScroller).not.toBeNull();
    expect(keysScroller).not.toBe(configScroller);
    expect(keysScroller).toHaveProperty("scrollTop", 0);
  });

  it("updates the URL for link and programmatic navigation", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await user.click(await screen.findByRole("link", { name: "Connections" }));
    await waitFor(() => expect(window.location.pathname).toBe("/connections/servers"));
    await user.click(screen.getByRole("button", { name: "open routed host" }));
    expect(window.location.pathname).toBe("/connections/servers");
    expect(window.location.search).toBe("?path=config&host=build01&panel=advanced&advanced=directives");
  });

  it("carries generated key identifiers to the next workflow without putting secrets in the URL", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/keys");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "hand key to connection" }));
    expect(await screen.findByText("preferred connection key id_new")).toBeInTheDocument();
    await waitFor(() => expect(window.location.pathname).toBe("/connections/servers"));
    expect(window.location.search).toBe("");
    await user.click(screen.getByRole("button", { name: "consume connection key" }));
    expect(screen.getByText("preferred connection key none")).toBeInTheDocument();

    await openFromMenu(user, "Keys");
    await user.click(await screen.findByRole("button", { name: "hand key to server" }));
    expect(await screen.findByText("remote keys panel id_new.pub")).toBeInTheDocument();
    expect(window.location.pathname).toBe("/install-key");
    await user.click(screen.getByRole("button", { name: "consume public key" }));
    await openFromMenu(user, "Keys");
    await openFromMenu(user, "Install Key on Server");
    expect(await screen.findByText("remote keys panel no key")).toBeInTheDocument();
  });

  it("passes the complete connection location through and clears it from the section link", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/connections/servers?path=config&host=bastion&panel=basic");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByText("connection location /connections/servers?path=config&host=bastion&panel=basic"))
      .toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "open routed host" }));
    expect(window.location.search).toBe("?path=config&host=build01&panel=advanced&advanced=directives");
    expect(screen.getByText("connection location /connections/servers?path=config&host=build01&panel=advanced&advanced=directives"))
      .toBeInTheDocument();

    await user.click(screen.getByRole("link", { name: "Connections" }));
    await waitFor(() => expect(window.location.pathname).toBe("/connections/servers"));
    expect(window.location.search).toBe("");
  });

  it("lets the connection editor block and later allow shell navigation", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/connections/servers?path=config&host=bastion&panel=basic");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "block connection navigation" }));
    await user.click(screen.getByRole("link", { name: "Menu" }));
    expect(window.location.pathname).toBe("/connections/servers");
    expect(screen.getByText("connections panel")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "allow connection navigation" }));
    await openFromMenu(user, "Keys");
    expect(window.location.pathname).toBe("/keys");
    expect(screen.getByText("keys panel")).toBeInTheDocument();
  });

  it("follows the real pathname on popstate", async () => {
    window.history.replaceState(null, "", "/keys");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );
    expect(await screen.findByText("keys panel")).toBeInTheDocument();

    act(() => {
      window.history.pushState({ section: "Connections" }, "", "/history");
      window.dispatchEvent(new PopStateEvent("popstate", { state: { section: "Connections" } }));
    });

    expect(screen.getByText("history panel")).toBeInTheDocument();
    expect(window.location.pathname).toBe("/history");
  });

  it("keeps a requested section while the vault is locked", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/connections");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 })}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "unlock fixture" }));

    expect(await screen.findByText("connections panel")).toBeInTheDocument();
    expect(window.location.pathname).toBe("/connections/servers");
  });

  it("shows and dismisses the version pair after an automatic vault migration", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], dedicatedKeyPassphrases: [] })}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "migrate fixture" }));
    expect(await screen.findByText(
      "The vault was safely migrated from version 4 to 5.",
    )).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Dismiss migration notice" }));
    expect(screen.queryByText("The vault was safely migrated from version 4 to 5.")).not.toBeInTheDocument();
  });

  it("remembers that a newly opened vault exists when it is locked again", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vi.fn().mockResolvedValue({
          exists: false,
          unlocked: false,
          aliases: [],
          dedicatedKeyPassphrases: [],
          minPassphraseLength: 12,
        })}
      />,
    );

    expect(await screen.findByText("new vault fixture")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "unlock fixture" }));
    await openFromMenu(user, "Secrets");
    await user.click(await screen.findByRole("button", { name: "lock fixture" }));

    expect(screen.getByText("existing vault fixture")).toBeInTheDocument();
  });

  it("keeps an unknown URL and links back to Home", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/missing");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    expect(await screen.findByRole("heading", { name: "Page not found", level: 2 })).toBeInTheDocument();
    expect(window.location.pathname).toBe("/missing");
    const home = screen.getByRole("link", { name: "Go to Home" });
    expect(home).toHaveAttribute("href", "/");

    await user.click(home);
    expect(window.location.pathname).toBe("/");
    expect(screen.getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  });

  it("translates the unknown-route recovery", async () => {
    window.history.replaceState(null, "", "/missing");
    render(
      <LanguageProvider initial="ja">
        <App
          bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
          health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
          vault={openVault}
        />
      </LanguageProvider>,
    );

    expect(await screen.findByRole("heading", { name: "ページが見つかりません", level: 2 })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "ホームへ移動" })).toHaveAttribute("href", "/");
  });

  it("switches to the keys panel", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
      />,
    );

    await openFromMenu(user, "Keys");

    expect(screen.getByText("keys panel")).toBeInTheDocument();
    expect(screen.getAllByRole("status")).toHaveLength(1);
  });

  it("keeps a non-secret connection draft across a prerequisite detour", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await user.click(await screen.findByRole("link", { name: "Connections" }));
    await user.click(screen.getByRole("button", { name: "open key prerequisite" }));

    expect(screen.getByText("keys panel")).toBeInTheDocument();
    expect(screen.getByText("Connection setup for lab-node is paused.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Return to connection setup" }));

    expect(screen.getByText("draft lab-node")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Connections" })).toHaveAttribute("aria-current", "page");
  });

  it("switches to the known hosts and ad hoc checks panels", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
      />,
    );

    await openFromMenu(user, "Known Hosts");
    expect(await screen.findByText("known hosts panel")).toBeInTheDocument();
    expect(screen.getAllByRole("status")).toHaveLength(1);

    await openFromMenu(user, "Ad hoc checks");
    expect(await screen.findByText("diagnostics panel")).toBeInTheDocument();
    expect(screen.getAllByRole("status")).toHaveLength(1);
  });

  it("switches to the remote keys panel", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
      />,
    );

    await openFromMenu(user, "Install Key on Server");

    expect(screen.getByText(/remote keys panel/)).toBeInTheDocument();
    expect(screen.getAllByRole("status")).toHaveLength(1);
  });

  it("switches to the history panel", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
      />,
    );

    await openFromMenu(user, "History");

    expect(screen.getByText("history panel")).toBeInTheDocument();
  });

  it("shows a recovery action when bootstrap fails", async () => {
    render(<App bootstrap={vi.fn().mockRejectedValue(new Error("rejected"))} health={vi.fn()} vault={openVault} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Secure local session could not be started. Restart sshc and use the newly opened tab.",
    );
  });

  it("shows a centered session-ended recovery screen when startup renewal says the session expired", async () => {
    render(<App bootstrap={vi.fn().mockRejectedValue(new Error("session_expired"))} health={vi.fn()} vault={openVault} />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Reload to renew the local session");
    expect(alert.closest("main")).toHaveClass("min-h-screen", "place-items-center");
    expect(screen.getByRole("button", { name: "Reload session" })).toBeVisible();
  });

  it("keeps a startup API 401 on the centered session-ended screen", async () => {
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vi.fn().mockRejectedValue(new ApiError("invalid_session", 401, null))}
      />,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("Reload to renew the local session");
    expect(screen.queryByText("Secure local session could not be started")).toBeNull();
  });

  it("replaces the ready shell with the session-ended screen after an API reports invalid_session", async () => {
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );
    await screen.findByRole("status", { name: "" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "invalid_session", message: "request rejected" }),
      { status: 401, headers: { "Content-Type": "application/problem+json" } },
    )));

    await act(async () => {
      await expect(apiClient.read("/api/v1/example")).rejects.toMatchObject({ code: "invalid_session" });
    });

    expect(screen.getByRole("alert")).toHaveTextContent("Reload to renew the local session");
    expect(screen.queryByRole("navigation", { name: "Primary" })).toBeNull();
  });

  it("uses a shared bootstrap exchange once when StrictMode re-runs effects", async () => {
    const exchange = vi.fn().mockResolvedValue({ csrfToken });
    const sessionPromise = exchange();
    const health = vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" });

    render(
      <StrictMode>
        <App bootstrap={() => sessionPromise} health={health} vault={openVault} />
      </StrictMode>,
    );

    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Local session active · 0.1.0"));
    expect(exchange).toHaveBeenCalledTimes(1);
    expect(health).toHaveBeenCalledTimes(1);
  });
  it("renders every panel inside the language provider", async () => {
    const user = userEvent.setup();
    render(
      <LanguageProvider initial="ja">
        <App
          bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
          health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
        />
      </LanguageProvider>,
    );

    expect(await screen.findByRole("status")).toHaveTextContent(ja["shell.active"].replace("{version}", "0.1.0"));
    await user.click(screen.getByRole("link", { name: ja["section.menu"] }));
    const menu = await screen.findByRole("region", { name: ja["section.menu"] });
    await user.click(within(menu).getByRole("link", { name: `${ja["section.keys"]}を開く` }));
    expect(screen.getByText("keys panel")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: ja["shell.primaryNavigation"] })).toBeInTheDocument();
  });

  it("switches language from the header and leaves the open section alone", async () => {
    const user = userEvent.setup();
    render(
      <LanguageProvider initial="en">
        <App
          bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
          health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
        />
      </LanguageProvider>,
    );

    await openFromMenu(user, "History");
    expect(screen.getByText("history panel")).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Lang"), "ja");

    expect(window.location.pathname).toBe("/history");
    expect(screen.getByText("history panel")).toBeInTheDocument();
  });
});

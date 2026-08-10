import { StrictMode, type ReactNode } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

// アプリケーションはマスターパスワードの向こうにあるため、シェルを期待する
// テストにはすべて開いた vault を与える必要がある。ロック画面には専用のテストがある。
const openVault = () =>
  Promise.resolve({ exists: true, unlocked: true, aliases: [] as string[], minPassphraseLength: 12 });
import { App } from "./App";
import { LanguageProvider } from "./i18n/context";
import { ThemeProvider } from "./theme/context";
import { ja } from "./i18n/messages";

vi.mock("./connections/ConnectionsPage", () => ({
  ConnectionsPage: ({
    onOpenFile,
    onInspector,
    creationDraft,
    onCreationDraftChange,
    onNavigateForCreation,
  }: {
    onOpenFile: (path: string, line: number) => void;
    onInspector: (content: { attention: boolean; body: ReactNode } | null) => void;
    creationDraft?: { alias: string } | null;
    onCreationDraftChange?: (draft: Record<string, string> | null) => void;
    onNavigateForCreation?: (section: "Groups" | "Keys") => void;
  }) => (
    <div>
      connections panel
      {creationDraft === null || creationDraft === undefined ? null : <span>{`draft ${creationDraft.alias}`}</span>}
      <button type="button" onClick={() => onOpenFile("config", 9)}>open pattern rule</button>
      <button type="button" onClick={() => onInspector({ attention: true, body: <p>inspector body</p> })}>
        offer inspector
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
    </div>
  ),
}));
vi.mock("./explorer/ConfigExplorer", () => ({
  ConfigExplorer: ({ target }: { target?: { path: string; line: number } | null }) => (
    <div>{`config panel ${target === null || target === undefined ? "no target" : `${target.path}:${target.line}`}`}</div>
  ),
}));
vi.mock("./groups/GroupsPanel", () => ({ GroupsPanel: () => <div>groups panel</div> }));
vi.mock("./history/HistoryPanel", () => ({ HistoryPanel: () => <div>history panel</div> }));
vi.mock("./keys/KeysScreen", () => ({ KeysScreen: () => <div>keys panel</div> }));
vi.mock("./diagnostics/DiagnosticsPanel", () => ({ DiagnosticsPanel: () => <div>diagnostics panel</div> }));
vi.mock("./knownhosts/KnownHostsPanel", () => ({ KnownHostsPanel: () => <div>known hosts panel</div> }));
vi.mock("./remotekeys/RemoteKeyPanel", () => ({ RemoteKeyPanel: () => <div>remote keys panel</div> }));
vi.mock("./secrets/LockScreen", () => ({
  LockScreen: ({ onOpen }: { onOpen: () => void }) => (
    <button type="button" onClick={onOpen}>unlock fixture</button>
  ),
}));

const csrfToken = "c".repeat(43);

afterEach(() => {
  window.history.replaceState(null, "", "/");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("App", () => {
  it("groups the navigation without adding headings to it", async () => {
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={openVault}
      />,
    );

    await screen.findByRole("heading", { name: "sshc" });

    // グループは見出しではなく名前付きリストである。ここに見出しを置くと
    // パネル自身の<h2>と衝突する。Playwright はアクセシブルネームを
    // 部分一致で照合するため、nav の見出し"Keys and hosts"は見出し
    // "Keys"を探すページレベルのクエリを二重にヒットさせてしまう。
    expect(screen.getByRole("list", { name: "Keys and hosts" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Keys and hosts" })).toBeNull();

    // どの section ボタンも元の名前のままである。
    for (const label of [
      "Home",
      "Connections",
      "Config",
      "Groups",
      "Keys",
      "Known Hosts",
      "Install Key on Server",
      "Ad hoc checks",
      "Secrets",
      "Sync",
      "History",
    ]) {
      expect(screen.getByRole("link", { name: label })).toHaveAttribute("href");
    }
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

    // 中身がなければトグルも出さない。常に提示されながら
    // 大抵空なペインは、人に開かせない習慣を教えてしまう。
    expect(screen.queryByRole("button", { name: /details/i })).toBeNull();

    await user.click(screen.getByRole("button", { name: "offer inspector" }));
    await user.click(screen.getByRole("button", { name: "Show details Needs attention" }));

    expect(screen.getByRole("complementary", { name: "Details" })).toHaveTextContent("inspector body");

    await user.click(screen.getByRole("link", { name: "Keys" }));
    await user.click(screen.getByRole("link", { name: "Connections" }));
    await user.click(screen.getByRole("button", { name: "offer inspector" }));

    expect(screen.getByRole("complementary", { name: "Details" })).toBeInTheDocument();
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
    expect(control).toHaveValue("system");

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
    for (const label of [
      "Home",
      "Connections",
      "Config",
      "Groups",
      "Keys",
      "Known Hosts",
      "Install Key on Server",
      "Ad hoc checks",
      "History",
    ]) {
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
    expect(screen.getByRole("link", { name: "Keys" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Connections" })).toHaveAttribute("href", "/connections");
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
    expect(window.location.pathname).toBe("/connections");
    await user.click(screen.getByRole("button", { name: "open pattern rule" }));
    expect(window.location.pathname).toBe("/config");
    expect(screen.getByText("config panel config:9")).toBeInTheDocument();
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
    expect(screen.getByRole("link", { name: "History" })).toHaveAttribute("aria-current", "page");
  });

  it("keeps a requested section while the vault is locked", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/connections");
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
        vault={vi.fn().mockResolvedValue({ exists: true, unlocked: false, aliases: [], minPassphraseLength: 12 })}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "unlock fixture" }));

    expect(await screen.findByText("connections panel")).toBeInTheDocument();
    expect(window.location.pathname).toBe("/connections");
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

    await user.click(await screen.findByRole("link", { name: "Keys" }));

    expect(screen.getByText("keys panel")).toBeInTheDocument();
    // ステータス領域を持つのはシェルだけであり、パネルが二つ目を追加してはならない。
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
    expect(screen.getByText("Connection setup for lab-node is waiting.")).toBeInTheDocument();
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

    await user.click(await screen.findByRole("link", { name: "Known Hosts" }));
    expect(screen.getByText("known hosts panel")).toBeInTheDocument();
    expect(screen.getAllByRole("status")).toHaveLength(1);

    await user.click(screen.getByRole("link", { name: "Ad hoc checks" }));
    expect(screen.getByText("diagnostics panel")).toBeInTheDocument();
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

    await user.click(await screen.findByRole("link", { name: "Install Key on Server" }));

    expect(screen.getByText("remote keys panel")).toBeInTheDocument();
    // ステータス領域を持つのはシェルだけであり、パネルが二つ目を追加してはならない。
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

    await user.click(await screen.findByRole("link", { name: "History" }));

    expect(screen.getByText("history panel")).toBeInTheDocument();
  });

  it("opens the config file view on the line a pattern rule asks for", async () => {
    const user = userEvent.setup();
    render(
      <App
        bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
        health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
      />,
    );

    await user.click(await screen.findByRole("link", { name: "Connections" }));
    await user.click(await screen.findByRole("button", { name: "open pattern rule" }));

    expect(screen.getByText("config panel config:9")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Config" })).toHaveAttribute("aria-current", "page");
    expect(screen.getAllByRole("status")).toHaveLength(1);
  });

  it("shows a recovery action when bootstrap fails", async () => {
    render(<App bootstrap={vi.fn().mockRejectedValue(new Error("rejected"))} health={vi.fn()} vault={openVault} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Secure local session could not be started. Restart sshc and use the newly opened tab.",
    );
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
  // パネルはプロバイダの外でレンダリングされると英語に翻訳される。これにより
  // コンポーネントテストが単体でレンダリングできるようになっている。だが
  // この便利さはここでは危険でもある。プロバイダのマウントをやめたシェルは
  // 英語のままでも正しく見えてしまい、他の全員にとっても英語のままになる。
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

    // シェル自身が翻訳を行う。
    expect(await screen.findByRole("status")).toHaveTextContent(ja["shell.active"].replace("{version}", "0.1.0"));
    expect(screen.getByRole("link", { name: ja["section.keys"] })).toBeInTheDocument();

    // 切り替え先の section も同じプロバイダ内にあるため、シェルを
    // 経由して到達したパネルも翻訳される。
    await user.click(screen.getByRole("link", { name: ja["section.keys"] }));
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

    await user.click(await screen.findByRole("link", { name: "History" }));
    expect(screen.getByText("history panel")).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Language"), "ja");

    // 変わったのはラベルであり、開いているパネルは変わっていない。section の
    // 識別子は section の名前ではない。
    expect(screen.getByRole("link", { name: ja["section.history"] })).toHaveAttribute("aria-current", "page");
    expect(screen.getByText("history panel")).toBeInTheDocument();
  });
});

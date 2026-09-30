import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DiagnosticsPanel } from "./DiagnosticsPanel";
import { LanguageProvider } from "../i18n/context";
import type { DiagnosticsApi, ReachabilityResponse } from "../api/diagnostics";

afterEach(() => {
  vi.restoreAllMocks();
});

function buildApi(overrides: Partial<DiagnosticsApi> = {}): DiagnosticsApi {
  return {
    configCheck: vi.fn().mockResolvedValue({
      root: "~/.ssh/config",
      files: [{ path: "~/.ssh/config", editable: true, missing: false, loads: 1, includes: 0 }],
      diagnostics: [],
    }),
    effective: vi.fn().mockResolvedValue({
      alias: "bastion",
      tokenWarning: "OpenSSH does not shell-escape the tokens it expands.",
      executableDirectives: [],
      values: [{ keyword: "hostname", values: ["203.0.113.10"] }],
      sources: [
        { keyword: "HostName", value: "203.0.113.10", path: "~/.ssh/config", line: 2, condition: "Host bastion", kind: "exact", winner: true },
      ],
      complexities: [],
      route: [],
    }),
    reachability: vi.fn().mockResolvedValue({
      address: "203.0.113.10:22",
      outcome: "reached",
      elapsedMs: 12,
      detail: "",
      // engine の notice は英語の定数で、画面は読まない。
      notice: "Engine notice that the screen does not show.",
    }),
    authentication: vi.fn().mockResolvedValue({
      outcome: "authenticated",
      authenticated: true,
      method: "publickey",
      detail: "",
      truncated: false,
      elapsedMs: 40,
    }),
    ...overrides,
  };
}

describe("DiagnosticsPanel", () => {
  it("explains that the source table scrolls sideways on narrow screens", async () => {
    const { container } = render(<DiagnosticsPanel api={buildApi()} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    const explain = screen.getByRole("button", { name: "Explain" });
    expect(container.firstElementChild).toHaveClass("[&_button]:min-h-10", "md:[&_button]:min-h-0");
    await userEvent.click(explain);

    const hint = await screen.findByText("Swipe sideways to see every column");
    expect(hint).toHaveClass("md:hidden");
    expect(screen.getByRole("table").parentElement).toHaveClass("overflow-x-auto");
  });

  it("runs no check until the user asks for one", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} />);

    await waitFor(() => expect(api.configCheck).toHaveBeenCalled());
    expect(api.effective).not.toHaveBeenCalled();
    expect(api.reachability).not.toHaveBeenCalled();
    expect(api.authentication).not.toHaveBeenCalled();
  });

  it("offers no check until an alias is typed", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} />);

    expect(screen.getByRole("button", { name: "Explain" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test authentication" })).toBeDisabled();

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");

    expect(screen.getByRole("button", { name: "Explain" })).toBeEnabled();
  });

  it("suggests saved aliases while still accepting an arbitrary SSH target", async () => {
    render(<DiagnosticsPanel api={buildApi()} hosts={["bastion", "database"]} />);

    const target = screen.getByLabelText("Host alias");
    expect(target).toHaveAttribute("list", "diagnostic-host-options");
    const suggestions = document.querySelectorAll<HTMLOptionElement>("#diagnostic-host-options option");
    expect([...suggestions].map((option) => option.value)).toEqual(["bastion", "database"]);

    await userEvent.type(target, "one-off.example.com");
    expect(target).toHaveValue("one-off.example.com");
    expect(screen.getByRole("button", { name: "Explain" })).toBeEnabled();
  });

  it("clears a standalone result when the typed target changes", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} hosts={["bastion", "nas"]} />);

    const target = screen.getByLabelText("Host alias");
    await userEvent.type(target, "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));
    expect(await screen.findByText(/203\.0\.113\.10:22/)).toBeInTheDocument();

    await userEvent.clear(target);
    await userEvent.type(target, "nas");
    expect(screen.queryByText(/203\.0\.113\.10:22/)).not.toBeInTheDocument();
  });

  it("drops a check's result when the typed target changes while the check runs", async () => {
    let answer: (value: ReachabilityResponse) => void = () => undefined;
    const api = buildApi({ reachability: vi.fn(() => new Promise<ReachabilityResponse>((resolve) => { answer = resolve; })) });
    render(<DiagnosticsPanel api={api} hosts={["bastion", "nas"]} />);

    const target = screen.getByLabelText("Host alias");
    await userEvent.type(target, "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));
    await userEvent.clear(target);
    await userEvent.type(target, "nas");
    await act(async () => answer({ address: "203.0.113.10:22", outcome: "reached", elapsedMs: 12, detail: "", notice: "" }));

    expect(screen.queryByText(/203\.0\.113\.10:22/)).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Reachability" })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Check reachability" })).toBeEnabled());
  });

  it("does not report a failure of a check for a target that was replaced while it ran", async () => {
    let fail: (reason: Error) => void = () => undefined;
    const api = buildApi({ reachability: vi.fn(() => new Promise<never>((_, reject) => { fail = reject; })) });
    render(<DiagnosticsPanel api={api} />);

    const target = screen.getByLabelText("Host alias");
    await userEvent.type(target, "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));
    await userEvent.clear(target);
    await userEvent.type(target, "nas");
    await act(async () => fail(new Error("api_mutation_failed")));

    await waitFor(() => expect(screen.getByRole("button", { name: "Check reachability" })).toBeEnabled());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("names the columns of the sources table", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Explain" }));

    expect(await screen.findByRole("columnheader", { name: "Keyword" })).toBeInTheDocument();
    for (const column of ["Value", "Read from", "Under", "State"]) {
      expect(screen.getByRole("columnheader", { name: column })).toBeInTheDocument();
    }
  });

  it("explains an alias and shows where each value came from", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Explain" }));

    await waitFor(() => expect(api.effective).toHaveBeenCalledWith("bastion"));
    expect(await screen.findByText("203.0.113.10")).toBeInTheDocument();
    expect(screen.getByText(/~\/\.ssh\/config:2/)).toBeInTheDocument();
  });


  it("says that a reachability check ignored ProxyJump", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));

    expect(await screen.findByText(/ProxyJump, ProxyCommand and any jump-host firewall were not used/)).toBeInTheDocument();
  });

  it("says that a connection with a VPN profile was not dialled and that authentication goes through the VPN", async () => {
    const api = buildApi({
      reachability: vi.fn().mockResolvedValue({
        address: "10.9.9.1:22",
        outcome: "not_checked",
        elapsedMs: 0,
        detail: "",
        notice: "Engine notice that the screen does not show.",
      }),
    });
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "lab");
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));

    expect(await screen.findByText("Not checked, because this connection goes through a VPN profile.")).toBeInTheDocument();
    expect(screen.getByText(/The authentication check connects through the VPN\./)).toBeInTheDocument();
    expect(screen.queryByText(/ProxyJump, ProxyCommand and any jump-host firewall/)).not.toBeInTheDocument();
  });

  it("says the check results in the screen's language instead of the engine's values", async () => {
    const api = buildApi({
      authentication: vi.fn().mockResolvedValue({
        outcome: "authentication_denied",
        authenticated: false,
        method: "",
        detail: "",
        truncated: false,
        elapsedMs: 40,
      }),
    });
    render(<LanguageProvider initial="ja"><DiagnosticsPanel api={api} /></LanguageProvider>);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    await userEvent.click(screen.getByRole("button", { name: "疎通を確認" }));
    expect(await screen.findByText("到達しました。ポートがTCP接続を受け付けました。")).toBeInTheDocument();
    expect(screen.getByText(/踏み台を経由しないと届かないホストはここで失敗します/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "認証を確認" }));
    expect(await screen.findByText(/認証が拒否されました/)).toBeInTheDocument();
    expect(screen.queryByText("reached")).not.toBeInTheDocument();
    expect(screen.queryByText("authentication_denied")).not.toBeInTheDocument();
    expect(screen.queryByText("Engine notice that the screen does not show.")).not.toBeInTheDocument();
  });

  it("reports a failed check without claiming success", async () => {
    const api = buildApi({
      reachability: vi.fn().mockRejectedValue(new Error("api_mutation_failed")),
    });
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/could not/i);
  });



  it("marks the rules it will not project instead of answering for them", async () => {
    const api = buildApi({
      effective: vi.fn().mockResolvedValue({
        alias: "lab",
        tokenWarning: "",
        executableDirectives: [],
        sources: [],
        complexities: [
          {
            code: "negated_pattern",
            path: "~/.ssh/config",
            line: 21,
            condition: "Host *,!lab",
            detail: "A negated pattern does not project onto a single host.",
          },
        ],
        route: [],
      }),
    });
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "lab");
    await userEvent.click(screen.getByRole("button", { name: "Explain" }));

    expect(await screen.findByText("This block excludes some hosts with a negated pattern (!).")).toBeInTheDocument();
    expect(screen.getByText(/~\/\.ssh\/config:21/)).toBeInTheDocument();
    expect(screen.getByText(/inside Host \*,!lab/)).toBeInTheDocument();
    expect(screen.queryByText("negated_pattern")).not.toBeInTheDocument();
    expect(screen.queryByText(/does not project onto a single host/)).not.toBeInTheDocument();
  });

  it("shows the multi-hop route and marks a hop it did not resolve", async () => {
    const api = buildApi({
      effective: vi.fn().mockResolvedValue({
        alias: "deep",
        tokenWarning: "",
        executableDirectives: [],
        sources: [],
        complexities: [],
        route: [
          { order: 0, depth: 0, parent: "", hop: "edge", hostname: "198.51.100.1", user: "ops", port: "22", complex: false },
          { order: 1, depth: 1, parent: "edge", hop: "inner-*", hostname: "", user: "", port: "", complex: true },
        ],
      }),
    });
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "deep");
    await userEvent.click(screen.getByRole("button", { name: "Explain" }));

    expect(await screen.findByText("edge")).toBeInTheDocument();
    expect(screen.getByText("ops@198.51.100.1:22")).toBeInTheDocument();
    expect(screen.getByText("inner-*")).toBeInTheDocument();
    expect(screen.getByText(/not a simple alias/)).toBeInTheDocument();
    expect(screen.getByText(/reached through edge/)).toBeInTheDocument();
  });

  it("keeps the lines that lost, because they are the answer to why not", async () => {
    const api = buildApi({
      effective: vi.fn().mockResolvedValue({
        alias: "bastion",
        tokenWarning: "",
        executableDirectives: [],
        sources: [
          { keyword: "Port", value: "2222", path: "~/.ssh/config", line: 4, condition: "Host bastion", kind: "exact", winner: true },
          { keyword: "Port", value: "22", path: "~/.ssh/config", line: 11, condition: "Host *", kind: "wildcard", winner: false },
        ],
        complexities: [],
        route: [],
      }),
    });
    render(<DiagnosticsPanel api={api} />);

    await userEvent.type(screen.getByLabelText("Host alias"), "bastion");
    await userEvent.click(screen.getByRole("button", { name: "Explain" }));

    const superseded = await screen.findByRole("row", { name: /Host \*/ });
    expect(within(superseded).getByText("22")).toBeInTheDocument();
    expect(within(superseded).getByText("superseded")).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /Host bastion/ })).getByText("in effect")).toBeInTheDocument();
  });

  it("shows the configuration diagnostics it already read", async () => {
    const api = buildApi({
      configCheck: vi.fn().mockResolvedValue({
        root: "~/.ssh/config",
        files: [{ path: "~/.ssh/config", editable: true, missing: false, loads: 1, includes: 1 }],
        diagnostics: [
          {
            severity: "error",
            code: "include_cycle",
            path: "~/.ssh/conf.d/10-home.conf",
            line: 3,
            detail: "This file includes itself.",
          },
        ],
      }),
    });
    render(<DiagnosticsPanel api={api} />);

    expect(await screen.findByText(/include_cycle/)).toBeInTheDocument();
    expect(screen.getByText(/This file includes itself/)).toBeInTheDocument();
  });

  it("diagnoses a fixed host without asking for an alias", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} host="bastion" />);

    expect(screen.queryByLabelText("Host alias")).not.toBeInTheDocument();
    expect(api.configCheck).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));

    await waitFor(() => expect(api.reachability).toHaveBeenCalledWith("bastion"));
    expect(await screen.findByText(/203\.0\.113\.10:22/)).toBeInTheDocument();
  });

  it("starts no check of its own when a fixed host is opened", async () => {
    const api = buildApi();
    render(<DiagnosticsPanel api={api} host="bastion" />);

    expect(api.effective).not.toHaveBeenCalled();
    expect(api.reachability).not.toHaveBeenCalled();
    expect(api.authentication).not.toHaveBeenCalled();
  });

  it("drops the previous host's results when the fixed host changes", async () => {
    const api = buildApi();
    const { rerender } = render(<DiagnosticsPanel api={api} host="bastion" />);

    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));
    expect(await screen.findByText(/203\.0\.113\.10:22/)).toBeInTheDocument();

    rerender(<DiagnosticsPanel api={api} host="nas" />);

    await waitFor(() => expect(screen.queryByText(/203\.0\.113\.10:22/)).not.toBeInTheDocument());
    expect(screen.queryByRole("heading", { name: "Reachability" })).not.toBeInTheDocument();
  });
});

import { useEffect } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import type { TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useTerminalSessions } from "../terminal/sessions";
import { useSplitView } from "../terminal/splitView/useSplitView";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { engineActionLog } from "../testing/engineActionLog";
import { TerminalScreen } from "./TerminalScreen";

const emptyList = { sessions: [], maxSessions: 50 };

function sessionsApi(terminalSessions: TerminalSessionsApi["terminalSessions"]): TerminalSessionsApi {
  return {
    terminalSessions,
    openTerminalSession: vi.fn(),
    reconnectTerminalSession: vi.fn(),
    stopTerminalReconnect: vi.fn(),
    closeTerminalSession: vi.fn(),
    setTerminalSessionTitle: vi.fn(),
    moveTerminalSession: vi.fn(),
    terminalStreamTicket: vi.fn(),
    streamPath: vi.fn(),
    terminalSessionDirectory: vi.fn(),
  } as unknown as TerminalSessionsApi;
}

function enginesApi(overrides: Partial<RemoteEnginesApi> = {}): RemoteEnginesApi {
  return {
    list: vi.fn(), add: vi.fn(), remove: vi.fn(), open: vi.fn(),
    status: vi.fn(async () => ({ alias: "r540", version: "v0.54.0", vault: true, unlocked: false, passwordless: false, sessions: 0 })),
    control: vi.fn(async () => ({ generation: 1 })),
    actionLog: vi.fn(async () => engineActionLog()),
    unlockVault: vi.fn(async () => undefined),
    ...overrides,
  };
}

function runTargets(selected: string | null, api = enginesApi()): RunTargetsState {
  return {
    engines: ["r540"], selected, problem: "", busy: false,
    select: vi.fn(), add: vi.fn(async () => true), remove: vi.fn(async () => undefined), openInNewTab: vi.fn(async () => undefined),
    api, localVersion: "v0.54.0",
    vaultUnlock: { unlockFor: null, revision: 0, ask: vi.fn(), close: vi.fn(), unlocked: vi.fn() },
    engineInfo: { openFor: null, ask: vi.fn(), close: vi.fn() },
    unread: new Set(),
  };
}

function Screen({ api, targets }: { api: TerminalSessionsApi; targets: RunTargetsState }) {
  const t = useTranslate();
  const terminalSessions = useTerminalSessions(api, t);
  const splitView = useSplitView({ sessions: terminalSessions.sessions, runTarget: targets.selected });
  return (
    <TerminalScreen
      visible
      terminalSessions={terminalSessions}
      activeSessionId={null}
      splitView={splitView}
      settings={{ appearance: { palette: "", font: "", background: "" } }}
      hostAppearance={new Map()}
      hostVPN={new Map()}
      api={api}
      runTargets={targets}
      onOpenRemotePath={vi.fn()}
      onOSC52Change={vi.fn(async () => undefined)}
    />
  );
}

function refusal(code: string, reason?: string, detail?: string) {
  return new ApiError(code, 502, {
    code, message: "request rejected",
    ...(reason === undefined ? {} : { reason }),
    ...(detail === undefined ? {} : { detail }),
  });
}

async function renderRefused(error: ApiError, targets = runTargets("r540")) {
  const terminalSessions = vi.fn().mockRejectedValue(error);
  render(<Screen api={sessionsApi(terminalSessions)} targets={targets} />);
  const panel = await screen.findByRole("region");
  return { panel, terminalSessions, targets };
}

function buttonNames(panel: HTMLElement): string[] {
  return within(panel).getAllByRole("button").map((button) => button.textContent ?? "");
}

afterEach(() => { vi.restoreAllMocks(); });

describe("TerminalScreen while the run target refuses its session list", () => {
  it.each([
    ["remote_engine_unreachable", undefined, "Could not connect to r540",
      "The engine terminals run on cannot be reached. Reconnect, or switch back to localhost."],
    ["remote_engine_not_running", undefined, "Could not connect to r540",
      "The sshc engine is not running on r540. Start it there with sshc service install or sshc open."],
    ["remote_engine_connect_failed", "host_key_unknown", "Could not connect to r540 over SSH",
      "The host key of r540 has not been confirmed. Connect to r540 once from a terminal first."],
    ["remote_engine_connect_failed", undefined, "Could not connect to r540 over SSH", null],
  ] as const)("shows %s (%s) with its heading, reason and actions", async (code, reason, heading, text) => {
    const { panel } = await renderRefused(refusal(code, reason));
    expect(panel).toHaveAccessibleName(heading);
    expect(within(panel).getByRole("heading", { name: heading })).toBeInTheDocument();
    if (text !== null) expect(within(panel).getByText(text)).toBeInTheDocument();
    expect(buttonNames(panel)).toEqual(["Reconnect now", "Switch back to localhost", "Details", "Engine information…"]);
    expect(within(panel).getByText("Tries again 2 s after each attempt ends")).toBeInTheDocument();
    expect(panel.querySelector("time")).not.toBeNull();
  });

  it("is not a dialog, so the rest of the page stays usable", async () => {
    await renderRefused(refusal("remote_engine_unreachable"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("reads the list again on Reconnect now and goes away once the engine answers", async () => {
    const user = userEvent.setup();
    const terminalSessions = vi.fn().mockRejectedValueOnce(refusal("remote_engine_unreachable")).mockResolvedValue(emptyList);
    render(<Screen api={sessionsApi(terminalSessions)} targets={runTargets("r540")} />);
    const panel = await screen.findByRole("region");
    await user.click(within(panel).getByRole("button", { name: "Reconnect now" }));
    expect(terminalSessions).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(screen.queryByRole("region")).not.toBeInTheDocument());
  });

  it("cannot be pressed again while the list is being read", async () => {
    const user = userEvent.setup();
    let answer: (value: typeof emptyList) => void = () => undefined;
    const terminalSessions = vi.fn()
      .mockRejectedValueOnce(refusal("remote_engine_unreachable"))
      .mockReturnValueOnce(new Promise((resolve) => { answer = resolve; }));
    render(<Screen api={sessionsApi(terminalSessions)} targets={runTargets("r540")} />);
    const panel = await screen.findByRole("region");
    const reconnect = within(panel).getByRole("button", { name: "Reconnect now" });
    await user.click(reconnect);
    expect(reconnect).toBeDisabled();
    expect(reconnect).toHaveAttribute("aria-busy", "true");
    answer(emptyList);
    await waitFor(() => expect(screen.queryByRole("region")).not.toBeInTheDocument());
  });

  it("shows the error code and reason under Details", async () => {
    const user = userEvent.setup();
    const { panel } = await renderRefused(refusal("remote_engine_connect_failed", "host_key_unknown", "dial tcp: i/o timeout"));
    const details = within(panel).getByRole("button", { name: "Details" });
    expect(details).toHaveAttribute("aria-expanded", "false");
    await user.click(details);
    expect(details).toHaveAttribute("aria-expanded", "true");
    expect(within(panel).getByText("remote_engine_connect_failed (host_key_unknown)")).toBeInTheDocument();
    expect(within(panel).getByText("dial tcp: i/o timeout")).toBeInTheDocument();
  });

  it("unlocks a locked Vault in the panel itself", async () => {
    const user = userEvent.setup();
    const api = enginesApi();
    const { panel, targets } = await renderRefused(refusal("remote_engine_vault_locked"), runTargets("r540", api));
    expect(panel).toHaveAccessibleName("The vault on r540 is locked");
    const password = await within(panel).findByLabelText("Vault password for r540");
    expect(password).not.toHaveFocus();
    await user.type(password, "secret");
    await user.click(within(panel).getByRole("button", { name: "Unlock" }));
    expect(api.unlockVault).toHaveBeenCalledWith("r540", "secret");
    expect(targets.vaultUnlock.unlocked).toHaveBeenCalled();
    expect(targets.vaultUnlock.ask).not.toHaveBeenCalled();
  });

  it("says in the panel when the Vault password is wrong", async () => {
    const user = userEvent.setup();
    const api = enginesApi({ unlockVault: vi.fn().mockRejectedValue(refusal("remote_engine_wrong_passphrase")) });
    const { panel, targets } = await renderRefused(refusal("remote_engine_vault_locked"), runTargets("r540", api));
    await user.type(await within(panel).findByLabelText("Vault password for r540"), "wrong");
    await user.click(within(panel).getByRole("button", { name: "Unlock" }));
    expect(await within(panel).findByText("The vault password for r540 is wrong.")).toBeInTheDocument();
    expect(targets.vaultUnlock.unlocked).not.toHaveBeenCalled();
  });

  it("updates an incompatible engine in the panel itself and reads the list again", async () => {
    const user = userEvent.setup();
    const api = enginesApi({ status: vi.fn().mockRejectedValue(refusal("remote_engine_incompatible", undefined, "v0.51.0")) });
    const { panel, terminalSessions } = await renderRefused(refusal("remote_engine_incompatible", undefined, "v0.51.0"), runTargets("r540", api));
    expect(panel).toHaveAccessibleName("The sshc on r540 does not work with this machine’s");
    expect(within(panel).getByText("v0.51.0")).toBeInTheDocument();
    expect(within(panel).getByText("v0.54.0")).toBeInTheDocument();
    const calls = terminalSessions.mock.calls.length;
    await user.click(within(panel).getByRole("button", { name: "Update r540" }));
    expect(api.control).toHaveBeenCalledWith("r540", "update");
    await waitFor(() => expect(terminalSessions.mock.calls.length).toBeGreaterThan(calls));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("switches back to this machine and opens engine information for the run target", async () => {
    const user = userEvent.setup();
    const { panel, targets } = await renderRefused(refusal("remote_engine_unreachable"));
    await user.click(within(panel).getByRole("button", { name: "Switch back to localhost" }));
    expect(targets.select).toHaveBeenCalledWith(null);
    await user.click(within(panel).getByRole("button", { name: "Engine information…" }));
    expect(targets.engineInfo.ask).toHaveBeenCalledWith("r540");
  });

  it("shows nothing for this machine's own list", async () => {
    const terminalSessions = vi.fn().mockRejectedValue(refusal("remote_engine_unreachable"));
    render(<Screen api={sessionsApi(terminalSessions)} targets={runTargets(null)} />);
    await waitFor(() => expect(terminalSessions).toHaveBeenCalled());
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });
});

// OpeningScreen opens a session on the run target as soon as it is drawn, as the
// sidebar does after switching to the terminal screen.
function OpeningScreen({ api, targets }: { api: TerminalSessionsApi; targets: RunTargetsState }) {
  const t = useTranslate();
  const terminalSessions = useTerminalSessions(api, t);
  const splitView = useSplitView({ sessions: terminalSessions.sessions, runTarget: targets.selected });
  const { open } = terminalSessions;
  useEffect(() => { void open({ kind: "ssh", alias: "db" }); }, [open]);
  return (
    <TerminalScreen
      visible
      terminalSessions={terminalSessions}
      activeSessionId={null}
      splitView={splitView}
      settings={{ appearance: { palette: "", font: "", background: "" } }}
      hostAppearance={new Map()}
      hostVPN={new Map()}
      api={api}
      runTargets={targets}
      onOpenRemotePath={vi.fn()}
      onOSC52Change={vi.fn(async () => undefined)}
    />
  );
}

describe("TerminalScreen while a session opens on a run target", () => {
  it("says the host is being reached and then why it could not be", async () => {
    let refuse: (error: unknown) => void = () => undefined;
    const api = sessionsApi(vi.fn().mockResolvedValue(emptyList));
    api.openTerminalSession = vi.fn(() => new Promise<never>((_, reject) => { refuse = reject; }));
    render(<OpeningScreen api={api} targets={runTargets("r540")} />);

    expect(await screen.findByRole("heading", { name: "Connecting to r540" })).toBeInTheDocument();
    refuse(refusal("remote_engine_unreachable"));

    const panel = await screen.findByRole("region");
    expect(panel).toHaveAccessibleName("Could not connect to r540");
    expect(screen.queryByText("Connecting to r540")).not.toBeInTheDocument();
  });
});

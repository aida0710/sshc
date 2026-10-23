import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useTerminalSessions } from "../terminal/sessions";
import { useSplitView } from "../terminal/splitView/useSplitView";
import type { RunTargetsState } from "../terminal/useRunTargets";
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

function runTargets(selected: string | null): RunTargetsState {
  return {
    engines: ["r540"], selected, problem: "", busy: false,
    select: vi.fn(), add: vi.fn(async () => true), remove: vi.fn(async () => undefined), openInNewTab: vi.fn(async () => undefined),
    api: {} as RunTargetsState["api"], localVersion: "v0.54.0",
    vaultUnlock: { unlockFor: null, revision: 0, ask: vi.fn(), offer: vi.fn(), allowOffer: vi.fn(), close: vi.fn(), unlocked: vi.fn() },
    engineInfo: { openFor: null, ask: vi.fn(), close: vi.fn() },
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
      hostOSC52={new Map()}
      hostVPN={new Map()}
      api={api}
      runTargets={targets}
      onOpenRemotePath={vi.fn()}
      onOSC52Change={vi.fn(async () => undefined)}
    />
  );
}

function refusal(code: string, reason?: string) {
  return new ApiError(code, 502, { code, message: "request rejected", ...(reason === undefined ? {} : { reason }) });
}

async function renderRefused(error: ApiError, targets = runTargets("r540")) {
  const terminalSessions = vi.fn().mockRejectedValue(error);
  render(<Screen api={sessionsApi(terminalSessions)} targets={targets} />);
  const panel = await screen.findByRole("dialog");
  return { panel, terminalSessions, targets };
}

function buttonNames(panel: HTMLElement): string[] {
  return within(panel).getAllByRole("button").map((button) => button.textContent ?? "");
}

afterEach(() => { vi.restoreAllMocks(); });

describe("TerminalScreen while the run target refuses its session list", () => {
  it.each([
    ["remote_engine_unreachable", undefined, "Could not connect to r540",
      "The engine terminals run on cannot be reached. Reconnect, or switch back to localhost.",
      ["Reconnect", "Switch back to localhost", "Engine information…"]],
    ["remote_engine_not_running", undefined, "Could not connect to r540",
      "The sshc engine is not running on r540. Start it there with sshc service install or sshc open.",
      ["Reconnect", "Switch back to localhost", "Engine information…"]],
    ["remote_engine_connect_failed", "host_key_unknown", "Could not connect to r540",
      "The host key of r540 has not been confirmed. Connect to r540 once from a terminal first.",
      ["Reconnect", "Switch back to localhost", "Engine information…"]],
    ["remote_engine_connect_failed", undefined, "Could not connect to r540", null,
      ["Reconnect", "Switch back to localhost", "Engine information…"]],
    ["remote_engine_vault_locked", undefined, "The vault on r540 is locked", null,
      ["Unlock…", "Reconnect", "Switch back to localhost"]],
    ["remote_engine_incompatible", undefined, "The engine on r540 runs an sshc version that does not work with this computer’s", null,
      ["Update…", "Reconnect", "Switch back to localhost"]],
  ] as const)("shows %s (%s) with its heading, reason and actions", async (code, reason, heading, text, buttons) => {
    const { panel } = await renderRefused(refusal(code, reason));
    expect(within(panel).getByRole("heading", { name: heading })).toBeInTheDocument();
    expect(panel).toHaveAccessibleName(heading);
    if (text === null) expect(panel.querySelector("p")).toBeNull();
    else expect(within(panel).getByText(text)).toBeInTheDocument();
    expect(buttonNames(panel)).toEqual(buttons);
  });

  it("reads the list again on Reconnect and goes away once the engine answers", async () => {
    const user = userEvent.setup();
    const terminalSessions = vi.fn().mockRejectedValueOnce(refusal("remote_engine_unreachable")).mockResolvedValue(emptyList);
    render(<Screen api={sessionsApi(terminalSessions)} targets={runTargets("r540")} />);
    const panel = await screen.findByRole("dialog");
    await user.click(within(panel).getByRole("button", { name: "Reconnect" }));
    expect(terminalSessions).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("cannot be pressed again while the list is being read", async () => {
    const user = userEvent.setup();
    let answer: (value: typeof emptyList) => void = () => undefined;
    const terminalSessions = vi.fn()
      .mockRejectedValueOnce(refusal("remote_engine_unreachable"))
      .mockReturnValueOnce(new Promise((resolve) => { answer = resolve; }));
    render(<Screen api={sessionsApi(terminalSessions)} targets={runTargets("r540")} />);
    const panel = await screen.findByRole("dialog");
    const reconnect = within(panel).getByRole("button", { name: "Reconnect" });
    await user.click(reconnect);
    expect(reconnect).toBeDisabled();
    expect(reconnect).toHaveAttribute("aria-busy", "true");
    answer(emptyList);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("switches back to this machine, unlocks and opens engine information for the run target", async () => {
    const user = userEvent.setup();
    const locked = await renderRefused(refusal("remote_engine_vault_locked"));
    await user.click(within(locked.panel).getByRole("button", { name: "Switch back to localhost" }));
    expect(locked.targets.select).toHaveBeenCalledWith(null);
    await user.click(within(locked.panel).getByRole("button", { name: "Unlock…" }));
    expect(locked.targets.vaultUnlock.ask).toHaveBeenCalledWith("r540");
  });

  it.each([
    ["remote_engine_incompatible", "Update…"],
    ["remote_engine_unreachable", "Engine information…"],
  ])("opens engine information from %s", async (code, button) => {
    const user = userEvent.setup();
    const { panel, targets } = await renderRefused(refusal(code));
    await user.click(within(panel).getByRole("button", { name: button }));
    expect(targets.engineInfo.ask).toHaveBeenCalledWith("r540");
  });

  it("shows nothing for this machine's own list", async () => {
    const terminalSessions = vi.fn().mockRejectedValue(refusal("remote_engine_unreachable"));
    render(<Screen api={sessionsApi(terminalSessions)} targets={runTargets(null)} />);
    await waitFor(() => expect(terminalSessions).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});

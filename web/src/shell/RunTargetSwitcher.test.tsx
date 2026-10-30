import { useState } from "react";
import { currentSettings } from "../settings/settingsStore";
import { fakeSettingsServer } from "../testing/settingsServer";
import { act, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { engineActionLog } from "../testing/engineActionLog";
import { useRemoteVaultUnlock, type RemoteVaultUnlockState } from "../terminal/useRemoteVaultUnlock";
import { useRunTargets, type RunTargetsState } from "../terminal/useRunTargets";
import { EngineInfoDialog } from "./EngineInfoDialog";
import { RunTargetSwitcher } from "./RunTargetSwitcher";

const translate = ((key: string, values?: Record<string, string>) =>
  values === undefined ? key : `${key} ${Object.values(values).join(" ")}`) as never;

function enginesApi(overrides: Partial<RemoteEnginesApi> = {}): RemoteEnginesApi {
  return {
    list: vi.fn(async () => ({ engines: [{ alias: "r540", connected: true }] })),
    add: vi.fn(async () => undefined),
    remove: vi.fn(async () => ({ engines: [] })),
    status: vi.fn(),
    open: vi.fn(async () => ({ url: "http://127.0.0.1:61000/#bootstrap=x" })),
    control: vi.fn(async () => ({ generation: 1 })),
    actionLog: vi.fn(async () => engineActionLog()),
    unlockVault: vi.fn(async () => undefined),
    ...overrides,
  };
}

function targets(overrides: Partial<RunTargetsState> = {}): RunTargetsState {
  return {
    engines: ["r540"], selected: null, problem: "", busy: false,
    select: vi.fn(), add: vi.fn(async () => true), remove: vi.fn(async () => undefined), openInNewTab: vi.fn(async () => undefined),
    api: enginesApi(), localVersion: "v0.53.0", vaultUnlock: vaultUnlockState(),
    engineInfo: { openFor: null, ask: vi.fn(), close: vi.fn() },
    unread: new Set(),
    ...overrides,
  };
}

function vaultUnlockState(overrides: Partial<RemoteVaultUnlockState> = {}): RemoteVaultUnlockState {
  return {
    unlockFor: null, revision: 0,
    ask: vi.fn(), close: vi.fn(), unlocked: vi.fn(),
    ...overrides,
  };
}

afterEach(() => vi.restoreAllMocks());

function incompatibleEngine() {
  return new ApiError("remote_engine_incompatible", 409, { code: "remote_engine_incompatible", message: "request rejected", detail: "v0.51.0" });
}

function SwitcherWithDialogs({ state }: { state: RunTargetsState }) {
  const [openFor, setOpenFor] = useState<string | null>(null);
  const engineInfo = { openFor, ask: setOpenFor, close: () => setOpenFor(null) };
  return <RunTargetSwitcher targets={{ ...state, engineInfo }} aliases={["r540"]} hosts={[]} />;
}

describe("useRemoteVaultUnlock", () => {
  it("closes the dialog and counts the unlock once the Vault is unlocked", () => {
    const { result } = renderHook(() => useRemoteVaultUnlock());
    act(() => result.current.ask("r540"));
    expect(result.current.unlockFor).toBe("r540");
    act(() => result.current.unlocked());
    expect(result.current.unlockFor).toBeNull();
    expect(result.current.revision).toBe(1);
  });
});

describe("useRunTargets", () => {
  it("remembers the chosen engine and falls back to this machine when it is no longer added", async () => {
    fakeSettingsServer();
    const api = enginesApi();
    const { result } = renderHook(() => useRunTargets({ translate, enabled: true, localVersion: "v1", api }));
    await waitFor(() => expect(result.current.engines).toEqual(["r540"]));
    act(() => result.current.select("r540"));
    expect(result.current.selected).toBe("r540");
    expect(currentSettings()?.state.interface?.runTarget).toBe("r540");

    await act(async () => { await result.current.remove("r540"); });
    expect(result.current.selected).toBeNull();
  });

  it("opens engine information when opening a run target reports an incompatible engine", async () => {
    const closeTab = vi.fn();
    vi.spyOn(window, "open").mockReturnValue({ close: closeTab } as unknown as Window);
    const open = vi.fn().mockRejectedValue(incompatibleEngine());
    const api = enginesApi({ open });
    const { result } = renderHook(() => useRunTargets({ translate, enabled: true, localVersion: "v0.53.0", api }));
    await act(async () => { await result.current.openInNewTab("r540"); });
    expect(result.current.problem).toBe("");
    expect(result.current.engineInfo.openFor).toBe("r540");
    expect(closeTab).toHaveBeenCalled();
  });

});

describe("RunTargetSwitcher", () => {
  it("marks the switch and the engine whose terminals sent a notification nobody has seen", async () => {
    const user = userEvent.setup();
    render(<RunTargetSwitcher targets={targets({ unread: new Set(["r540"]) })} aliases={["r540"]} hosts={[]} />);
    const trigger = screen.getByRole("button", { name: /^Choose where terminals run/ });
    expect(within(trigger).getByText("Another engine has new notifications")).toBeInTheDocument();
    await user.click(trigger);
    const list = screen.getByRole("list", { name: "Engines" });
    expect(within(list).getByRole("button", { name: /^r540.*New notifications/ })).toBeInTheDocument();
  });

  it("chooses an engine from the list that opens upward", async () => {
    const user = userEvent.setup();
    const state = targets();
    render(<RunTargetSwitcher targets={state} aliases={["r540"]} hosts={[]} />);
    await user.click(screen.getByRole("button", { name: /^Choose where terminals run/ }));
    const list = screen.getByRole("list", { name: "Engines" });
    await user.click(within(list).getByRole("button", { name: /^r540/ }));
    expect(state.select).toHaveBeenCalledWith("r540");
  });

  it("offers opening the full screen in a new tab and removing an engine, not this machine", async () => {
    const user = userEvent.setup();
    const state = targets();
    render(<RunTargetSwitcher targets={state} aliases={["r540"]} hosts={[]} />);
    await user.click(screen.getByRole("button", { name: /^Choose where terminals run/ }));
    expect(screen.getAllByRole("button", { name: /^Actions for engine/ })).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Actions for engine r540" }));
    await user.click(screen.getByRole("button", { name: "Open its full screen in a new tab" }));
    expect(state.openInNewTab).toHaveBeenCalledWith("r540");
    await user.click(screen.getByRole("button", { name: "Remove from the list" }));
    expect(state.remove).toHaveBeenCalledWith("r540");
  });

  it("shows the engine's version, Vault and sessions, and restarts it after saying its sessions end", async () => {
    // A compatible engine of another version answers its status; only the
    // version line says that this machine differs.
    const user = userEvent.setup();
    const control = vi.fn(async () => ({ generation: 1 }));
    const actionLog = vi.fn(async () => engineActionLog({ action: "restart", output: "restarted sshc.service" }));
    const status = vi.fn(async () => ({ alias: "r540", version: "v0.51.0", vault: true, unlocked: false, passwordless: false, sessions: 3 }));
    const state = targets({ api: enginesApi({ control, actionLog, status }) });
    render(<SwitcherWithDialogs state={state} />);
    await user.click(screen.getByRole("button", { name: /^Choose where terminals run/ }));
    await user.click(screen.getByRole("button", { name: "Actions for engine r540" }));
    await user.click(screen.getByRole("button", { name: "Engine information…" }));

    const dialog = await screen.findByRole("dialog", { name: "Engine on r540" });
    await within(dialog).findByText("v0.51.0");
    expect(within(dialog).getByText("this machine runs v0.53.0")).toBeInTheDocument();
    expect(within(dialog).queryByText(/Cannot connect to r540/)).not.toBeInTheDocument();
    expect(within(dialog).getByText("locked")).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Restart" }));
    const confirm = screen.getByRole("dialog", { name: "Restart the engine on r540?" });
    expect(within(confirm).getByText("All 3 sessions running on r540 will end. A password-protected vault will lock.")).toBeInTheDocument();
    await user.click(within(confirm).getByRole("button", { name: "Restart" }));

    expect(control).toHaveBeenCalledWith("r540", "restart");
    expect(await screen.findByRole("status")).toHaveTextContent("Restarted.");
    expect(screen.getByRole("log", { name: "Output of sshc on that host" })).toHaveTextContent("restarted sshc.service");
  });

  it("unlocks the run target's Vault with the password and says when it is wrong", async () => {
    const user = userEvent.setup();
    const unlockVault = vi.fn(async (_alias: string, passphrase: string) => {
      if (passphrase !== "correct horse battery") {
        throw new ApiError("remote_engine_wrong_passphrase", 403, { code: "remote_engine_wrong_passphrase", message: "request rejected" });
      }
    });
    const vaultUnlock = vaultUnlockState({ unlockFor: "r540" });
    const status = vi.fn(async () => ({ alias: "r540", version: "v0.53.0", vault: true, unlocked: false, passwordless: false, sessions: 2 }));
    render(<RunTargetSwitcher targets={targets({ api: enginesApi({ unlockVault, status }), vaultUnlock })} aliases={["r540"]} hosts={[]} />);

    const dialog = screen.getByRole("dialog", { name: "Unlock the vault on r540" });
    const field = within(dialog).getByLabelText("Vault password for r540");
    await waitFor(() => expect(field).toBeEnabled());
    await user.type(field, "wrong");
    await user.click(within(dialog).getByRole("button", { name: "Unlock" }));
    expect(await within(dialog).findByText("The vault password for r540 is wrong.")).toBeInTheDocument();
    expect(vaultUnlock.unlocked).not.toHaveBeenCalled();

    await user.clear(field);
    await user.type(field, "correct horse battery{Enter}");
    await waitFor(() => expect(vaultUnlock.unlocked).toHaveBeenCalledTimes(1));
    expect(unlockVault).toHaveBeenLastCalledWith("r540", "correct horse battery");
  });

  it("sends an engine with no Vault yet to its own screen instead of asking for a password", async () => {
    const user = userEvent.setup();
    const status = vi.fn(async () => ({ alias: "r540", version: "v0.53.0", vault: false, unlocked: false, passwordless: false, sessions: 0 }));
    const state = targets({ api: enginesApi({ status }), vaultUnlock: vaultUnlockState({ unlockFor: "r540" }) });
    render(<RunTargetSwitcher targets={state} aliases={["r540"]} hosts={[]} />);

    const dialog = await screen.findByRole("dialog", { name: "r540 has no vault" });
    await user.click(within(dialog).getByRole("button", { name: "Open its full screen in a new tab" }));
    expect(state.vaultUnlock.close).toHaveBeenCalled();
    expect(state.openInNewTab).toHaveBeenCalledWith("r540");
  });
});

describe("EngineInfoDialog", () => {
  it("shows both versions and clears the update prompt once the updated engine answers", async () => {
    const user = userEvent.setup();
    const status = vi.fn().mockRejectedValueOnce(incompatibleEngine()).mockResolvedValue({
      alias: "r540", version: "v0.53.0", vault: true, unlocked: true, passwordless: true, sessions: 0,
    });
    const api = enginesApi({ status });
    render(<EngineInfoDialog alias="r540" targets={targets({ api })} onClose={vi.fn()} />);
    const dialog = screen.getByRole("dialog", { name: "Engine on r540" });
    const prompt = "Cannot connect to r540 because its sshc version does not work with this machine’s. Update it?";
    expect(await within(dialog).findByText(prompt)).toBeInTheDocument();
    expect(within(dialog).getAllByText("Version")).toHaveLength(1);
    expect(within(dialog).getByText("v0.51.0")).toBeInTheDocument();
    expect(within(dialog).getByText("this machine runs v0.53.0")).toBeInTheDocument();
    expect(within(dialog).queryByText("Vault")).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Update" }));
    const confirmation = screen.getByRole("dialog", { name: "Update sshc on r540?" });
    expect(within(confirmation).getByText(/Every session/)).toBeInTheDocument();
    await user.click(within(confirmation).getByRole("button", { name: "Update" }));
    await waitFor(() => expect(within(dialog).queryByText(prompt)).not.toBeInTheDocument());
    expect(api.control).toHaveBeenCalledWith("r540", "update");
    expect(within(dialog).getAllByText("Version")).toHaveLength(1);
    expect(within(dialog).getByText("v0.53.0")).toBeInTheDocument();
    expect(within(dialog).getByText("Vault")).toBeInTheDocument();
  });

  it("names the sessions on the run target that stopping it ends, and keeps to the count when they cannot be read", async () => {
    const user = userEvent.setup();
    const status = vi.fn(async () => ({ alias: "r540", version: "v0.53.0", vault: true, unlocked: true, passwordless: true, sessions: 1 }));
    const running = { id: "one", kind: "ssh", alias: "osaka", workspaceId: "default", title: "deploy", startedAt: "2026-10-28T09:00:00Z", state: "connected", problem: "" } as const;
    const terminalSessions = vi.fn().mockResolvedValueOnce({ sessions: [running], maxSessions: 12 }).mockRejectedValueOnce(new Error("vault_locked"));
    const terminalSessionsOf = vi.fn(() => ({ terminalSessions }));
    render(<EngineInfoDialog alias="r540" targets={targets({ api: enginesApi({ status }) })} onClose={vi.fn()} terminalSessionsOf={terminalSessionsOf} />);
    const dialog = screen.getByRole("dialog", { name: "Engine on r540" });
    await within(dialog).findByText("v0.53.0");

    await user.click(within(dialog).getByRole("button", { name: "Stop" }));
    let confirmation = screen.getByRole("dialog", { name: /^Stop the engine on r540/ });
    const listed = await within(confirmation).findByRole("list", { name: "Sessions that end" });
    expect(within(listed).getAllByRole("listitem").map((row) => row.textContent)).toEqual(["deployosaka"]);
    expect(terminalSessionsOf).toHaveBeenCalledWith("r540");
    await user.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    await user.click(within(dialog).getByRole("button", { name: "Stop" }));
    confirmation = screen.getByRole("dialog", { name: /^Stop the engine on r540/ });
    await waitFor(() => expect(terminalSessions).toHaveBeenCalledTimes(2));
    expect(within(confirmation).queryByRole("list")).not.toBeInTheDocument();
    expect(within(confirmation).getByText("All 1 sessions running on r540 will end.")).toBeInTheDocument();
    expect(within(confirmation).getByRole("button", { name: "Stop" })).toBeEnabled();
  });

  it("keeps the version prompt and allows another update when the update fails", async () => {
    const user = userEvent.setup();
    const api = enginesApi({ status: vi.fn().mockRejectedValue(incompatibleEngine()), actionLog: vi.fn().mockResolvedValue(engineActionLog({ succeeded: false, output: "update failed" })) });
    render(<EngineInfoDialog alias="r540" targets={targets({ api })} onClose={vi.fn()} />);
    const dialog = screen.getByRole("dialog", { name: "Engine on r540" });
    await within(dialog).findByText("v0.51.0");
    await user.click(within(dialog).getByRole("button", { name: "Update" }));
    await user.click(within(screen.getByRole("dialog", { name: "Update sshc on r540?" })).getByRole("button", { name: "Update" }));
    expect(await screen.findByRole("status")).toHaveTextContent("failed");
    expect(within(dialog).getByText(/Cannot connect to r540/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Update" })).toBeEnabled();
  });
});

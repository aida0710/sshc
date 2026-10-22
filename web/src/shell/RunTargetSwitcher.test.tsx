import { act, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useRunTargets, type RunTargetsState } from "../terminal/useRunTargets";
import { RunTargetSwitcher } from "./RunTargetSwitcher";

const translate = ((key: string, values?: Record<string, string>) =>
  values === undefined ? key : `${key} ${Object.values(values).join(" ")}`) as never;

function enginesApi(overrides: Partial<RemoteEnginesApi> = {}): RemoteEnginesApi {
  return {
    list: vi.fn(async () => ({ engines: [{ alias: "r540" }] })),
    add: vi.fn(async () => undefined),
    remove: vi.fn(async () => ({ engines: [] })),
    status: vi.fn(),
    open: vi.fn(async () => ({ url: "http://127.0.0.1:61000/#bootstrap=x" })),
    control: vi.fn(async () => ({ succeeded: true, output: "" })),
    ...overrides,
  };
}

function targets(overrides: Partial<RunTargetsState> = {}): RunTargetsState {
  return {
    engines: ["r540"], selected: null, problem: "", busy: false,
    select: vi.fn(), add: vi.fn(async () => true), remove: vi.fn(async () => undefined), openInNewTab: vi.fn(async () => undefined),
    api: enginesApi(), localVersion: "v0.53.0",
    ...overrides,
  };
}

afterEach(() => window.localStorage.clear());

describe("useRunTargets", () => {
  it("remembers the chosen engine and falls back to this machine when it is no longer added", async () => {
    const api = enginesApi();
    const { result } = renderHook(() => useRunTargets({ translate, enabled: true, localVersion: "v1", api }));
    await waitFor(() => expect(result.current.engines).toEqual(["r540"]));
    act(() => result.current.select("r540"));
    expect(result.current.selected).toBe("r540");
    expect(window.localStorage.getItem("sshc.terminal.run-target.v1")).toBe("r540");

    await act(async () => { await result.current.remove("r540"); });
    expect(result.current.selected).toBeNull();
  });

  it("says which version a refused engine runs and what to do on that host", async () => {
    const add = vi.fn(async () => {
      throw new ApiError("remote_engine_version_mismatch", 409, { code: "remote_engine_version_mismatch", message: "request rejected", detail: "v0.51.0" });
    });
    const api = enginesApi({ add });
    const { result } = renderHook(() => useRunTargets({ translate, enabled: true, localVersion: "v0.53.0", api }));
    let added = true;
    await act(async () => { added = await result.current.add("gpu-box"); });
    expect(added).toBe(false);
    expect(result.current.problem).toBe("runTarget.versionMismatch gpu-box v0.51.0 v0.53.0");
    expect(result.current.selected).toBeNull();
  });
});

describe("RunTargetSwitcher", () => {
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
    const user = userEvent.setup();
    const control = vi.fn(async () => ({ succeeded: true, output: "restarted sshc.service" }));
    const status = vi.fn(async () => ({ alias: "r540", version: "v0.51.0", vault: true, unlocked: false, passwordless: false, sessions: 3 }));
    const state = targets({ api: enginesApi({ control, status }) });
    render(<RunTargetSwitcher targets={state} aliases={["r540"]} hosts={[]} />);
    await user.click(screen.getByRole("button", { name: /^Choose where terminals run/ }));
    await user.click(screen.getByRole("button", { name: "Actions for engine r540" }));
    await user.click(screen.getByRole("button", { name: "Engine information…" }));

    const dialog = await screen.findByRole("dialog", { name: "Engine on r540" });
    await within(dialog).findByText("v0.51.0");
    expect(within(dialog).getByText("this computer runs v0.53.0")).toBeInTheDocument();
    expect(within(dialog).getByText("locked")).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Restart" }));
    const confirm = screen.getByRole("dialog", { name: "Restart the engine on r540?" });
    expect(within(confirm).getByText("All 3 sessions running on r540 will end. A password-protected vault will lock.")).toBeInTheDocument();
    await user.click(within(confirm).getByRole("button", { name: "Restart" }));

    expect(control).toHaveBeenCalledWith("r540", "restart");
    expect(await screen.findByRole("status")).toHaveTextContent("Restarted.");
    expect(screen.getByLabelText("Output of sshc on that host")).toHaveTextContent("restarted sshc.service");
  });
});

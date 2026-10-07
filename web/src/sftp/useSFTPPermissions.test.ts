import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { RemoteEntry } from "./api";
import type { ChmodPlan } from "./chmodApi";
import type { SFTPBrowserModel } from "./useSFTPBrowser";
import { useSFTPPermissions } from "./useSFTPPermissions";

const permissions = vi.hoisted(() => ({ plan: vi.fn(), apply: vi.fn() }));
const files = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("./chmodApi", async (importOriginal) => ({
  ...await importOriginal<typeof import("./chmodApi")>(), chmodApi: permissions,
}));
vi.mock("./api", () => ({ sftpApi: files }));

const options = { fileMode: "644", directoryMode: "755", recursive: false };
const plan: ChmodPlan = { revision: "plan-revision", selectionCount: 1, files: 1, directories: 0, skippedSymlinks: 0, options, actionToken: "b".repeat(43), actionExpiresAt: "2026-10-07T12:01:00Z" };
const entry: RemoteEntry = { name: "file", path: "/srv/file", type: "file", size: 1, mode: "-rw-------", modifiedAt: "2026-10-07T12:00:00Z", revision: "original-revision" };

function permissionFixture() {
  const isCurrent = vi.fn(() => true);
  const browser = { alias: "edge", path: "/srv", source: { can: { chmod: true } }, generation: { observe: () => isCurrent }, setProblem: vi.fn() } as unknown as SFTPBrowserModel;
  const refreshAfterChange = vi.fn(async () => undefined);
  const offerUndo = vi.fn<(label: string, run: () => Promise<void>) => void>();
  const rendered = renderHook(() => useSFTPPermissions({ browser, refreshAfterChange, offerUndo, onInteract: vi.fn() }));
  return { ...rendered, browser, isCurrent, refreshAfterChange, offerUndo };
}

describe("permissions confirmation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    permissions.plan.mockResolvedValue(plan);
    permissions.apply.mockResolvedValue({ applied: 1, items: 1, complete: true });
    files.list.mockResolvedValue({ path: "/srv", entries: [{ ...entry, revision: "updated-revision", mode: "-rw-r--r--" }] });
  });

  it("keeps the confirmed host, selection and options when the browser or caller changes them", async () => {
    const fixture = permissionFixture();
    const selected = { ...entry };
    const selectedOptions = { ...options };
    act(() => fixture.result.current.ask([selected]));
    selected.path = "/another";
    selected.revision = "another-revision";
    await act(() => fixture.result.current.review(selectedOptions));
    selectedOptions.fileMode = "777";
    fixture.browser.alias = "another-host";
    fixture.isCurrent.mockReturnValue(false);
    await act(() => fixture.result.current.apply());
    expect(permissions.apply).toHaveBeenCalledWith({ alias: "edge", plan, selection: {
      entries: [{ path: "/srv/file", expectedRevision: "original-revision" }], options,
    } });
    expect(fixture.refreshAfterChange).not.toHaveBeenCalled();
  });

  it("discards a pending plan after cancellation without reopening a modal or changing permissions", async () => {
    let resolvePlan: (plan: ChmodPlan) => void = () => { throw new Error("plan request not started"); };
    permissions.plan.mockImplementation(() => new Promise<ChmodPlan>((resolve) => { resolvePlan = resolve; }));
    const fixture = permissionFixture();
    act(() => fixture.result.current.ask([entry]));
    act(() => { void fixture.result.current.review(options); });
    expect(fixture.result.current.planning).toBe(true);
    act(() => fixture.result.current.cancel());
    await act(async () => { resolvePlan(plan); });
    expect(fixture.result.current.intent).toBeNull();
    expect(fixture.result.current.confirmation).toBeNull();
    expect(permissions.apply).not.toHaveBeenCalled();
  });

  it.each([new Error("connection lost"), new ApiError("request_failed", 502, null)])("does not replay a consumed confirmation after an uncertain response (%s)", async (failure) => {
    permissions.apply.mockRejectedValue(failure);
    const fixture = permissionFixture();
    act(() => fixture.result.current.ask([entry]));
    await act(() => fixture.result.current.review(options));
    await act(() => fixture.result.current.apply());
    expect(fixture.result.current.problem).toContain("Some changes may have been applied.");
    await act(() => fixture.result.current.apply());
    expect(permissions.apply).toHaveBeenCalledTimes(1);
    expect(fixture.result.current.attempted).toBe(true);
  });

  it("explains that a preflight refusal changed nothing", async () => {
    permissions.apply.mockRejectedValue(new ApiError("sftp_conflict", 409, null));
    const fixture = permissionFixture();
    act(() => fixture.result.current.ask([entry]));
    await act(() => fixture.result.current.review(options));
    await act(() => fixture.result.current.apply());
    expect(fixture.result.current.problem).toContain("No permissions were changed.");
    expect(fixture.result.current.problem).not.toContain("Some changes may have been applied.");
  });

  it("preserves single-entry undo with a fresh revision and another mandatory confirmation", async () => {
    const fixture = permissionFixture();
    act(() => fixture.result.current.ask([entry]));
    await act(() => fixture.result.current.review(options));
    await act(() => fixture.result.current.apply());
    const undo = fixture.offerUndo.mock.calls[0]?.[1];
    expect(undo).toBeDefined();
    await act(async () => { await undo?.(); });
    expect(fixture.result.current.intent?.initialMode).toBe("600");
    expect(fixture.result.current.intent?.entries[0]?.revision).toBe("updated-revision");
    expect(fixture.result.current.confirmation).toBeNull();
    expect(permissions.apply).toHaveBeenCalledTimes(1);
  });
});

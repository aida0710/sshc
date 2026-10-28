import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostDetail } from "../api/config";
import { vaultStatus } from "../testing/vaultStatus";
import { HostDetailPanel } from "./HostDetail";
import type { ConnectionSavedState } from "./connectionSavedState";
import type { HostDetailApi } from "./HostDetail";

const detail: HostDetail = {
  form: {
    entry: {
      identity: { path: "config", alias: "bastion" },
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      line: 1,
      patterns: ["bastion"],
      editable: true,
    },
    fields: [
      { line: 2, keyword: "HostName", values: ["203.0.113.10"], category: "basic", editable: true },
      { line: 3, keyword: "ProxyJump", values: ["edge"], category: "jump", editable: true },
      { line: 4, keyword: "Compression", values: ["yes"], category: "advanced", editable: true },
    ],
    raw: "Host bastion\n\tHostName 203.0.113.10\n\tProxyJump edge\n",
    comment: "",
    commentLines: 0,
  },
  metadata: { identity: { path: "config", alias: "bastion" } },
  effective: {
    alias: "bastion",

    entries: [{ keyword: "HostName", values: ["203.0.113.10"], source: { path: "config", line: 2 } }],
  },
  file: {
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    contents: "Host bastion\n\tHostName 203.0.113.10\n\tProxyJump edge\n",
    digest: "digest",
    editable: true,
    exists: true,
  },
};

function savedState(): ConnectionSavedState {
  return {
    detail,
    keys: { status: "ready", value: [] },
    vault: { status: "ready", value: vaultStatus() },
    credentials: { status: "ready", value: [] },
    eligibility: { status: "ready", value: { alias: "bastion", storable: true, blockers: [], warnings: [] } },
  };
}

function integrations(): HostDetailApi {
  return {
    effective: vi.fn().mockResolvedValue({
      alias: "bastion", executableDirectives: [], sources: [],
    }),
    reachability: vi.fn(),
    authentication: vi.fn(),
    passwordVault: vi.fn(),
    credentials: vi.fn(),
    passwordEligibility: vi.fn(),
    initialiseVault: vi.fn(),
    unlockVault: vi.fn(),
  } as unknown as HostDetailApi;
}

function renderPanel(overrides: Partial<Parameters<typeof HostDetailPanel>[0]> = {}) {
  const props = {
    detail,
    savedState: savedState(),
    preview: null,
    problem: null,
    onFieldEdits: vi.fn(),
    onBlockRaw: vi.fn(),
    onBasicSave: vi.fn().mockResolvedValue(undefined),
    onMetadataSave: vi.fn().mockResolvedValue(undefined),
    integrations: integrations(),
    onDirtyChange: vi.fn(),
    ...overrides,
  };
  const rendered = render(<HostDetailPanel {...props} />);
  return { props, ...rendered };
}

describe("HostDetailPanel", () => {
  it("uses the four direct route panels without running checks", async () => {
    const user = userEvent.setup();
    const onLocationChange = vi.fn();
    const harness = renderPanel({ panel: "Basic", advanced: "Settings", onLocationChange });

    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    expect(within(areaTabs).getAllByRole("tab")).toHaveLength(4);
    expect(areaTabs).not.toHaveClass("sticky");
    expect(within(areaTabs).getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
    expect(within(areaTabs).getByRole("tab", { name: "Basic" })).toHaveClass("border-accent");
    expect(screen.getByRole("tabpanel", { name: "Basic" })).toBeVisible();
    const editor = screen.getByRole("tabpanel", { name: "Basic" }).closest("[data-connection-editor]");
    const reachability = screen.getByRole("button", { name: "Check reachability" });
    expect(editor).not.toBeNull();
    expect(editor).not.toHaveClass("sshc-card", "rounded-lg");
    expect(screen.getByRole("region", { name: "Authentication" })).toContainElement(screen.getByRole("region", { name: "Connection checks" }));
    expect(screen.getByRole("region", { name: "Connection checks" })).toContainElement(reachability);
    expect(reachability).toBeEnabled();
    expect(harness.props.integrations.reachability).not.toHaveBeenCalled();

    await user.click(within(areaTabs).getByRole("tab", { name: "Resolved values" }));
    expect(onLocationChange).toHaveBeenCalledWith("Analysis", "Settings");

    await user.click(within(areaTabs).getByRole("tab", { name: "Display & terminal" }));
    expect(onLocationChange).toHaveBeenCalledWith("Sshc", "Settings");
  });

  it("supports the WAI-ARIA keyboard pattern for editor tabs", async () => {
    const user = userEvent.setup();
    const onLocationChange = vi.fn();
    const harness = renderPanel({ panel: "Basic", advanced: "Settings", onLocationChange });
    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    const tabs = within(areaTabs).getAllByRole("tab");

    expect(tabs.map((tab) => tab.tabIndex)).toEqual([0, -1, -1, -1]);
    tabs[0]?.focus();
    await user.keyboard("{ArrowRight}");
    expect(tabs[1]).toHaveFocus();
    expect(onLocationChange).toHaveBeenLastCalledWith("Advanced", "Settings");

    harness.rerender(<HostDetailPanel {...harness.props} panel="Advanced" advanced="Settings" />);
    expect(tabs.map((tab) => tab.tabIndex)).toEqual([-1, 0, -1, -1]);
    await user.keyboard("{End}");
    expect(tabs[3]).toHaveFocus();
    expect(onLocationChange).toHaveBeenLastCalledWith("Analysis", "Settings");
    await user.keyboard("{Home}");
    expect(tabs[0]).toHaveFocus();
    expect(onLocationChange).toHaveBeenLastCalledWith("Basic", "Settings");
    await user.keyboard("{ArrowLeft}");
    expect(tabs[3]).toHaveFocus();
    expect(onLocationChange).toHaveBeenLastCalledWith("Analysis", "Settings");
  });

  it("shows connection-specific app settings in the sshc panel", () => {
    renderPanel({ panel: "Sshc", advanced: "Settings" });

    expect(screen.getByRole("tabpanel", { name: "Display & terminal" })).toBeVisible();
    expect(screen.getByLabelText("Remote text encoding")).toBeVisible();
  });

  it("keeps a Basic draft mounted across areas and blocks connection checks", async () => {
    const user = userEvent.setup();
    const harness = renderPanel({ panel: "Basic", advanced: "Settings" });
    const hostName = screen.getByLabelText("Host name or IP address");
    await user.clear(hostName);
    await user.type(hostName, "198.51.100.7");

    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeDisabled();

    harness.rerender(<HostDetailPanel {...harness.props} panel="Analysis" advanced="Settings" />);
    expect(screen.getByRole("region", { name: "Settings analysis" })).toBeVisible();
    harness.rerender(<HostDetailPanel {...harness.props} panel="Basic" advanced="Settings" />);
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("198.51.100.7");
  });

  // 接続の確認は、認証の設定のすぐ下で試すものなので、基本タブの認証の節の最後にだけ置く。
  it("shows the connection checks at the end of the Basic area's authentication only", () => {
    const harness = renderPanel({ panel: "Basic", advanced: "Settings" });
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeVisible();
    for (const panel of ["Analysis", "Advanced", "Sshc"] as const) {
      harness.rerender(<HostDetailPanel {...harness.props} panel={panel} advanced="Settings" />);
      expect(screen.queryByRole("region", { name: "Connection checks" })).not.toBeInTheDocument();
    }
  });

  // sshc タブの項目も、Basic と同じく枠で囲まずに編集欄へ直接並べる。
  it("lays out the sshc settings without a surrounding panel, like the Basic area", () => {
    renderPanel({ panel: "Sshc", advanced: "Settings" });
    const panel = screen.getByRole("tabpanel", { name: "Display & terminal" });
    for (let element = screen.getByLabelText("Remote text encoding").parentElement; element !== null && element !== panel; element = element.parentElement) {
      expect(element).not.toHaveClass("bg-tree");
    }
  });

  it("keeps an sshc draft mounted across areas and locks the other editors until it is saved", async () => {
    const user = userEvent.setup();
    const harness = renderPanel({ panel: "Sshc", advanced: "Settings" });
    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");

    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);
    expect(harness.props.onMetadataSave).not.toHaveBeenCalled();

    harness.rerender(<HostDetailPanel {...harness.props} panel="Basic" advanced="Settings" />);
    expect(screen.getByLabelText("Host name or IP address")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeDisabled();
    harness.rerender(<HostDetailPanel {...harness.props} panel="Sshc" advanced="Settings" />);
    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("shift_jis");

    await user.click(screen.getByRole("button", { name: "Save sshc-only settings" }));
    expect(harness.props.onMetadataSave).toHaveBeenCalledWith(expect.objectContaining({ encoding: "shift_jis" }));
  });

  it("locks the sshc settings while Basic has a draft", async () => {
    const user = userEvent.setup();
    const harness = renderPanel({ panel: "Basic", advanced: "Settings" });
    await user.type(screen.getByLabelText("Host name or IP address"), "7");

    harness.rerender(<HostDetailPanel {...harness.props} panel="Sshc" advanced="Settings" />);
    expect(screen.getByLabelText("Remote text encoding")).toBeDisabled();
  });

  it("hands the page one discard for the drafts of every area", async () => {
    const user = userEvent.setup();
    let discard: (() => void) | null = null;
    const harness = renderPanel({
      panel: "Sshc",
      advanced: "Settings",
      onDiscardReady: (next: (() => void) | null) => { discard = next; },
    });
    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);

    act(() => (discard as unknown as () => void)());

    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("");
    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(false);
  });

  it("keeps the Advanced mark on while its draft survives a reload of the same file", async () => {
    const user = userEvent.setup();
    const harness = renderPanel({ panel: "Advanced", advanced: "Raw", savedRevision: 1 });
    const raw = screen.getByLabelText(/Block text/);
    await user.type(raw, "\tPort 2200\n");
    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);

    harness.rerender(<HostDetailPanel {...harness.props} panel="Advanced" advanced="Raw" savedRevision={2} />);

    expect(screen.getByLabelText(/Block text/)).toHaveValue(`${detail.form.raw}\tPort 2200\n`);
    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);
  });

  it("shows the Host block's text when the route asks for it, and goes back to the settings", async () => {
    const user = userEvent.setup();
    const onLocationChange = vi.fn();
    renderPanel({ panel: "Advanced", advanced: "Raw", onLocationChange });

    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    expect(within(areaTabs).getByRole("tab", { name: "Advanced" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText(/Block text/)).toHaveValue(detail.form.raw);
    await user.click(screen.getByRole("button", { name: "Back to the settings" }));
    expect(onLocationChange).toHaveBeenCalledWith("Advanced", "Settings");
  });
});

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostDetail } from "../api/config";
import type { IntegrationsApi } from "../api/integrations";
import { HostDetailPanel } from "./HostDetail";
import type { ConnectionSavedState } from "./connectionSavedState";

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
  metadata: { identity: { path: "config", alias: "bastion" }, favourite: false },
  effective: {
    alias: "bastion",
    approximate: true,
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
    vault: {
      status: "ready",
      value: { exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [], minPassphraseLength: 12 },
    },
    credentials: { status: "ready", value: [] },
    eligibility: { status: "ready", value: { alias: "bastion", storable: true, blockers: [], warnings: [] } },
  };
}

function integrations(): IntegrationsApi {
  return {
    effective: vi.fn().mockResolvedValue({
      alias: "bastion", evaluated: true, tokenWarning: "", executableDirectives: [], sources: [],
      failure: { failed: false, exitCode: 0, stderr: "" },
    }),
    reachability: vi.fn(),
    authentication: vi.fn(),
    passwordVault: vi.fn(),
    credentials: vi.fn(),
    passwordEligibility: vi.fn(),
    initialiseVault: vi.fn(),
    unlockVault: vi.fn(),
  } as unknown as IntegrationsApi;
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
    integrations: integrations(),
    onDirtyChange: vi.fn(),
    ...overrides,
  };
  const rendered = render(<HostDetailPanel {...props} />);
  return { props, ...rendered };
}

describe("HostDetailPanel", () => {
  it("folds legacy route tabs into three connection areas without running checks", async () => {
    const user = userEvent.setup();
    const onTabChange = vi.fn();
    const harness = renderPanel({ tab: "Diagnostics", onTabChange });

    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    expect(within(areaTabs).getAllByRole("tab")).toHaveLength(3);
    expect(within(areaTabs).getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeEnabled();
    expect(harness.props.integrations.reachability).not.toHaveBeenCalled();

    await user.click(within(areaTabs).getByRole("tab", { name: "Settings analysis" }));
    expect(onTabChange).toHaveBeenCalledWith("Effective");
  });

  it("keeps a Basic draft mounted across areas and blocks connection checks", async () => {
    const user = userEvent.setup();
    const harness = renderPanel({ tab: "Basic" });
    const hostName = screen.getByLabelText("Host name or IP address");
    await user.clear(hostName);
    await user.type(hostName, "198.51.100.7");

    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeDisabled();

    harness.rerender(<HostDetailPanel {...harness.props} tab="Effective" />);
    expect(screen.getByRole("region", { name: "Settings analysis" })).toBeVisible();
    harness.rerender(<HostDetailPanel {...harness.props} tab="Basic" />);
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("198.51.100.7");
  });

  it("maps old advanced URLs to the matching internal advanced view", () => {
    renderPanel({ tab: "Raw" });

    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    expect(within(areaTabs).getByRole("tab", { name: "Advanced settings" })).toHaveAttribute("aria-selected", "true");
    const advancedTabs = screen.getByRole("tablist", { name: "Advanced setting views" });
    expect(within(advancedTabs).getByRole("tab", { name: "Raw" })).toHaveAttribute("aria-selected", "true");
  });

  it("reports the legacy route corresponding to an advanced subview", async () => {
    const user = userEvent.setup();
    const onTabChange = vi.fn();
    renderPanel({ tab: "Jump", onTabChange });

    await user.click(screen.getByRole("tab", { name: "Directives" }));
    expect(onTabChange).toHaveBeenCalledWith("Advanced");
  });
});

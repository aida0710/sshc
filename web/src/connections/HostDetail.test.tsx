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
      alias: "bastion", tokenWarning: "", executableDirectives: [], sources: [],
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
  it("uses the three direct route panels without running checks", async () => {
    const user = userEvent.setup();
    const onLocationChange = vi.fn();
    const harness = renderPanel({ panel: "Basic", advanced: "Jump", onLocationChange });

    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    expect(within(areaTabs).getAllByRole("tab")).toHaveLength(3);
    expect(within(areaTabs).getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
    expect(within(areaTabs).getByRole("tab", { name: "Basic" })).toHaveClass("border-accent");
    expect(screen.getByRole("tabpanel", { name: "Basic" })).toBeVisible();
    const editor = screen.getByRole("tabpanel", { name: "Basic" }).closest("[data-connection-editor]");
    const reachability = screen.getByRole("button", { name: "Check reachability" });
    expect(editor).not.toBeNull();
    expect(editor).not.toHaveClass("sshc-card", "rounded-lg");
    expect(editor).not.toContainElement(reachability);
    expect(reachability.closest("section")).toHaveClass("bg-card");
    expect(reachability).toBeEnabled();
    expect(harness.props.integrations.reachability).not.toHaveBeenCalled();

    await user.click(within(areaTabs).getByRole("tab", { name: "Analysis" }));
    expect(onLocationChange).toHaveBeenCalledWith("Analysis", "Jump");
  });

  it("keeps a Basic draft mounted across areas and blocks connection checks", async () => {
    const user = userEvent.setup();
    const harness = renderPanel({ panel: "Basic", advanced: "Jump" });
    const hostName = screen.getByLabelText("Host name or IP address");
    await user.clear(hostName);
    await user.type(hostName, "198.51.100.7");

    expect(harness.props.onDirtyChange).toHaveBeenLastCalledWith(true);
    expect(screen.getByRole("button", { name: "Check reachability" })).toBeDisabled();

    harness.rerender(<HostDetailPanel {...harness.props} panel="Analysis" advanced="Jump" />);
    expect(screen.getByRole("region", { name: "Settings analysis" })).toBeVisible();
    harness.rerender(<HostDetailPanel {...harness.props} panel="Basic" advanced="Jump" />);
    expect(screen.getByLabelText("Host name or IP address")).toHaveValue("198.51.100.7");
  });

  it("shows the advanced subview named directly by the route", () => {
    renderPanel({ panel: "Advanced", advanced: "Raw" });

    const areaTabs = screen.getByRole("tablist", { name: "Connection editor" });
    expect(within(areaTabs).getByRole("tab", { name: "Advanced" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("combobox", { name: "Advanced setting views" })).toHaveValue("Raw");
  });

  it("reports an advanced subview as a direct canonical location", async () => {
    const user = userEvent.setup();
    const onLocationChange = vi.fn();
    renderPanel({ panel: "Advanced", advanced: "Jump", onLocationChange });

    await user.selectOptions(screen.getByRole("combobox", { name: "Advanced setting views" }), "Directives");
    expect(onLocationChange).toHaveBeenCalledWith("Advanced", "Directives");
  });
});

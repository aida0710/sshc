import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { EffectiveResponse, IntegrationsApi } from "../api/integrations";
import { ConnectionChecks } from "./ConnectionChecks";

const safeInspection: EffectiveResponse = {
  alias: "bastion",
  tokenWarning: "OpenSSH does not shell-escape expanded tokens.",
  executableDirectives: [],
  sources: [],
  complexities: [],
  route: [],
};

function api(overrides: Partial<Pick<IntegrationsApi, "effective" | "reachability" | "authentication">> = {}) {
  return {
    effective: vi.fn().mockResolvedValue(safeInspection),
    reachability: vi.fn().mockResolvedValue({
      address: "203.0.113.10:22",
      outcome: "reached",
      elapsedMs: 12,
      detail: "",
      notice: "Direct destination check.",
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
  } as Pick<IntegrationsApi, "effective" | "reachability" | "authentication">;
}

describe("ConnectionChecks", () => {
  it("starts no network or process action when it is mounted", () => {
    const integrations = api();
    render(<ConnectionChecks alias="bastion" api={integrations} disabled={false} resetKey={0} />);

    expect(integrations.effective).not.toHaveBeenCalled();
    expect(integrations.reachability).not.toHaveBeenCalled();
    expect(integrations.authentication).not.toHaveBeenCalled();
  });

  it("runs reachability only after the explicit action", async () => {
    const integrations = api();
    render(<ConnectionChecks alias="bastion" api={integrations} disabled={false} resetKey={0} />);

    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));

    expect(await screen.findByText("203.0.113.10:22")).toBeInTheDocument();
    expect(integrations.effective).not.toHaveBeenCalled();
    expect(integrations.authentication).not.toHaveBeenCalled();
  });

  it("preflights saved settings and authenticates directly when no directive can execute", async () => {
    const integrations = api();
    render(<ConnectionChecks alias="bastion" api={integrations} disabled={false} resetKey={0} />);

    await userEvent.click(screen.getByRole("button", { name: "Check authentication with saved settings" }));

    expect(await screen.findByText("authenticated")).toBeInTheDocument();
    expect(integrations.effective).toHaveBeenCalledWith("bastion");
    expect(integrations.authentication).toHaveBeenCalledWith("bastion", false);
  });

  it("requires a second explicit acknowledgement when authentication can execute a directive", async () => {
    const risky: EffectiveResponse = {
      ...safeInspection,
      executableDirectives: [{
        keyword: "ProxyCommand",
        command: "/usr/bin/nc %h %p",
        path: "config",
        line: 9,
        condition: "Host bastion",
        onEvaluate: false,
        onConnect: true,
        overridable: false,
      }],
    };
    const integrations = api({ effective: vi.fn().mockResolvedValue(risky) });
    render(<ConnectionChecks alias="bastion" api={integrations} disabled={false} resetKey={0} />);

    await userEvent.click(screen.getByRole("button", { name: "Check authentication with saved settings" }));

    expect(await screen.findByText("ProxyCommand at config:9")).toBeInTheDocument();
    expect(screen.getByText("/usr/bin/nc %h %p")).toBeInTheDocument();
    expect(integrations.authentication).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Acknowledge and check authentication" }));
    expect(await screen.findByText("authenticated")).toBeInTheDocument();
    expect(integrations.authentication).toHaveBeenCalledWith("bastion", true);
  });

  it("clears results when the connection or saved revision changes", async () => {
    const integrations = api();
    const rendered = render(
      <ConnectionChecks alias="bastion" api={integrations} disabled={false} resetKey={0} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Check reachability" }));
    expect(await screen.findByText("203.0.113.10:22")).toBeInTheDocument();

    rendered.rerender(
      <ConnectionChecks alias="database" api={integrations} disabled={false} resetKey={1} />,
    );

    await waitFor(() => expect(screen.queryByText("203.0.113.10:22")).not.toBeInTheDocument());
  });
});

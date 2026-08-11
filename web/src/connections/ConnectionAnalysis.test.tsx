import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostDetail } from "../api/config";
import type { EffectiveResponse, IntegrationsApi } from "../api/integrations";
import { ConnectionAnalysis } from "./ConnectionAnalysis";

const detail: HostDetail = {
  form: {
    entry: {
      identity: { path: "config", alias: "bastion" },
      file: { path: "config", absolute: "/home/tester/.ssh/config" },
      line: 1,
      patterns: ["bastion"],
      editable: true,
    },
    fields: [],
    raw: "Host bastion\n",
    comment: "",
    commentLines: 0,
  },
  metadata: { identity: { path: "config", alias: "bastion" }, favourite: false },
  effective: {
    alias: "bastion",
    approximate: true,
    entries: [{ keyword: "HostName", values: ["203.0.113.10"], source: { path: "config", line: 2 } }],
    notices: [{ code: "explained_values_only", path: "config", line: 1, detail: "OpenSSH is authoritative." }],
  },
  file: {
    file: { path: "config", absolute: "/home/tester/.ssh/config" },
    contents: "Host bastion\n",
    digest: "digest",
    editable: true,
    exists: true,
  },
};

const effective: EffectiveResponse = {
  alias: "bastion",
  evaluated: true,
  requiresConfirmation: false,
  tokenWarning: "OpenSSH token warning.",
  executableDirectives: [],
  values: [{ keyword: "hostname", values: ["203.0.113.10"] }],
  sources: [{
    keyword: "HostName",
    value: "203.0.113.10",
    path: "config",
    line: 2,
    condition: "Host bastion",
    kind: "exact",
    winner: true,
  }],
  complexities: [],
  route: [],
  failure: { failed: false, exitCode: 0, stderr: "", truncated: false },
};

describe("ConnectionAnalysis", () => {
  it("shows explained saved values without running OpenSSH", () => {
    const api = { effective: vi.fn() } as Pick<IntegrationsApi, "effective">;
    render(<ConnectionAnalysis detail={detail} alias="bastion" api={api} />);

    expect(screen.getByText("HostName 203.0.113.10")).toBeInTheDocument();
    expect(screen.getByText("config:2")).toBeInTheDocument();
    expect(screen.getByText(/These values explain what this engine reads/)).toBeInTheDocument();
    expect(api.effective).not.toHaveBeenCalled();
  });

  it("runs authoritative ssh -G only after the explicit action", async () => {
    const api = { effective: vi.fn().mockResolvedValue(effective) };
    render(<ConnectionAnalysis detail={detail} alias="bastion" api={api} />);

    await userEvent.click(screen.getByRole("button", { name: "Run authoritative ssh -G" }));

    expect(api.effective).toHaveBeenCalledWith("bastion", false);
    expect(await screen.findByRole("table", { name: "Authoritative value sources" })).toBeInTheDocument();
    expect(screen.getByText("in effect")).toBeInTheDocument();
  });

  it("requires explicit confirmation before ssh -G can execute Match exec", async () => {
    const risky: EffectiveResponse = {
      ...effective,
      evaluated: false,
      requiresConfirmation: true,
      executableDirectives: [{
        keyword: "Match exec",
        command: "/usr/local/bin/check-network",
        path: "config",
        line: 12,
        condition: "Match exec",
        onEvaluate: true,
        onConnect: false,
        overridable: false,
      }],
    };
    const api = {
      effective: vi.fn()
        .mockResolvedValueOnce(risky)
        .mockResolvedValueOnce({ ...effective, evaluated: true }),
    };
    render(<ConnectionAnalysis detail={detail} alias="bastion" api={api} />);

    await userEvent.click(screen.getByRole("button", { name: "Run authoritative ssh -G" }));
    expect(await screen.findByText("/usr/local/bin/check-network")).toBeInTheDocument();
    expect(api.effective).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Run ssh -G with these directives" }));
    expect(api.effective).toHaveBeenLastCalledWith("bastion", true);
  });
});

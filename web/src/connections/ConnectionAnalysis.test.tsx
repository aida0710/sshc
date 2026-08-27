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
  metadata: { identity: { path: "config", alias: "bastion" } },
  effective: {
    alias: "bastion",

    entries: [{ keyword: "HostName", values: ["203.0.113.10"], source: { path: "config", line: 2 } }],
    notices: [],
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
  tokenWarning: "OpenSSH token warning.",
  executableDirectives: [],
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
};

describe("ConnectionAnalysis", () => {
  it("keeps the sources view disabled while another editor is dirty", () => {
    const api = { effective: vi.fn() } as Pick<IntegrationsApi, "effective">;
    render(<ConnectionAnalysis detail={detail} alias="bastion" api={api} disabled />);

    expect(screen.getByRole("button", { name: "Show the sources" })).toBeDisabled();
  });

  it("shows the resolved saved values without running anything", () => {
    const api = { effective: vi.fn() } as Pick<IntegrationsApi, "effective">;
    render(<ConnectionAnalysis detail={detail} alias="bastion" api={api} />);

    expect(screen.getByText("HostName 203.0.113.10")).toBeInTheDocument();
    expect(screen.getByText("config:2")).toBeInTheDocument();
    expect(screen.getByText(/These are the values used by this connection/)).toBeInTheDocument();
    expect(api.effective).not.toHaveBeenCalled();
  });

  it("reads the sources only after the explicit action", async () => {
    const api = { effective: vi.fn().mockResolvedValue(effective) };
    render(<ConnectionAnalysis detail={detail} alias="bastion" api={api} />);

    await userEvent.click(screen.getByRole("button", { name: "Show the sources" }));

    expect(api.effective).toHaveBeenCalledWith("bastion");
    expect(await screen.findByRole("table", { name: "Configuration lines read by OpenSSH" })).toBeInTheDocument();
    expect(screen.getByText("in effect")).toBeInTheDocument();
  });

});

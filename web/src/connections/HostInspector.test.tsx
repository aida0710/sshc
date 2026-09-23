import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { HostInspector } from "./HostInspector";
import type { HostDetail } from "../api/config";
import type { VPNProfile } from "../api/vpn";

// vpnProfile は、接続先 target へ届く WireGuard のプロファイルである。
function vpnProfile(name: string, target: string): VPNProfile {
  return {
    name,
    backend: "wireguard",
    target,
    wireguard: { server: "vpn.example.jp:51820", peerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=", address: "10.9.9.2/32" },
  };
}

// build() の接続は HostName も Port も持たないので、相手は alias の bastion:22 になる。
const reachingProfiles = [vpnProfile("tohoku", "bastion:22"), vpnProfile("office", "BASTION.:22")];

function build(): HostDetail {
  return {
    form: {
      entry: {
        identity: { path: "connections/work/bastion.conf", alias: "bastion" },
        file: {
          path: "connections/work/bastion.conf",
          absolute: "/home/tester/.ssh/connections/work/bastion.conf",
        },
        line: 1,
        patterns: ["bastion"],
        group: "work",
        editable: true,
      },
      fields: [],
      raw: "Host bastion\n",
      comment: "",
      commentLines: 0,
      notices: [],
    },
    metadata: { identity: { path: "connections/work/bastion.conf", alias: "bastion" } },
    effective: { alias: "bastion", entries: [], notices: [] },
    file: {
      file: {
        path: "connections/work/bastion.conf",
        absolute: "/home/tester/.ssh/connections/work/bastion.conf",
      },
      contents: "Host bastion\n",
      digest: "digest",
      editable: true,
      exists: true,
    },
  } as HostDetail;
}

describe("HostInspector", () => {
  it("changes the OS override and can return to automatic detection", async () => {
    const detail = build();
    const onMetadata = vi.fn();
    const { rerender } = render(<HostInspector detail={detail} onMetadata={onMetadata} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Operating system icon" }), "ubuntu");
    expect(onMetadata).toHaveBeenLastCalledWith({ ...detail.metadata, os: "ubuntu" });
    detail.metadata.os = "ubuntu";
    rerender(<HostInspector detail={detail} onMetadata={onMetadata} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Operating system icon" }), "");
    expect(onMetadata).toHaveBeenLastCalledWith({ ...detail.metadata, os: "" });
  });
  it("edits the display settings that live only in metadata", () => {
    const onMetadata = vi.fn();
    render(<HostInspector detail={build()} onMetadata={onMetadata} />);

    expect(screen.getByText(/saved immediately/)).toBeInTheDocument();
    expect(screen.getByLabelText(/^Tags/)).toBeInTheDocument();
    expect(screen.getByLabelText(/^Display order/)).toBeInTheDocument();
    expect(screen.getByLabelText("Colour")).toBeInTheDocument();
  });

  it("edits the display order the metadata schema has always carried", async () => {
    const onMetadata = vi.fn();
    const user = userEvent.setup();
    render(<HostInspector detail={build()} onMetadata={onMetadata} />);

    await user.type(screen.getByLabelText(/^Display order/), "7");

    expect(onMetadata).toHaveBeenLastCalledWith(expect.objectContaining({ order: 7 }));
  });

  it("saves the remote encoding per connection and can return to UTF-8", async () => {
    const onMetadata = vi.fn();
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(<HostInspector detail={detail} onMetadata={onMetadata} />);

    const encoding = screen.getByLabelText("Remote text encoding");
    await user.selectOptions(encoding, "shift_jis");
    expect(onMetadata).toHaveBeenLastCalledWith(expect.objectContaining({ encoding: "shift_jis" }));

    detail.metadata = { ...detail.metadata, encoding: "shift_jis" };
    rerender(<HostInspector detail={detail} onMetadata={onMetadata} />);
    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "");
    expect(onMetadata).toHaveBeenLastCalledWith(expect.not.objectContaining({ encoding: expect.anything() }));
  });

  it("stores an OSC 52 override per SSH connection and can inherit again", async () => {
    const onMetadata = vi.fn();
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(<HostInspector detail={detail} onMetadata={onMetadata} />);

    await user.selectOptions(screen.getByLabelText("OSC 52 clipboard"), "deny");
    expect(onMetadata).toHaveBeenLastCalledWith(expect.objectContaining({ osc52: "deny" }));

    detail.metadata = { ...detail.metadata, osc52: "deny" };
    rerender(<HostInspector detail={detail} onMetadata={onMetadata} />);
    await user.selectOptions(screen.getByLabelText("OSC 52 clipboard"), "");
    expect(onMetadata).toHaveBeenLastCalledWith(expect.not.objectContaining({ osc52: expect.anything() }));
  });

  it("routes this connection through a VPN profile and can stop routing it", async () => {
    const onMetadata = vi.fn();
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(
      <HostInspector detail={detail} onMetadata={onMetadata} vpnProfiles={reachingProfiles} />,
    );

    await user.selectOptions(screen.getByLabelText("VPN route"), "tohoku");
    expect(onMetadata).toHaveBeenLastCalledWith(expect.objectContaining({ vpn: "tohoku" }));

    detail.metadata = { ...detail.metadata, vpn: "tohoku" };
    rerender(<HostInspector detail={detail} onMetadata={onMetadata} vpnProfiles={reachingProfiles} />);
    await user.selectOptions(screen.getByLabelText("VPN route"), "");
    expect(onMetadata).toHaveBeenLastCalledWith(expect.not.objectContaining({ vpn: expect.anything() }));
  });

  it("still names a VPN profile that no longer exists, instead of showing no route", () => {
    const detail = build();
    detail.metadata = { ...detail.metadata, vpn: "retired" };

    render(<HostInspector detail={detail} onMetadata={vi.fn()} vpnProfiles={[vpnProfile("tohoku", "bastion:22")]} />);

    expect(screen.getByLabelText("VPN route")).toHaveValue("retired");
    expect(screen.getByRole("option", { name: "retired (profile is gone)" })).toBeInTheDocument();
  });

  it("does not offer a VPN profile whose target is another host, and says why", () => {
    render(
      <HostInspector
        detail={build()}
        onMetadata={vi.fn()}
        vpnProfiles={[vpnProfile("tohoku", "bastion:22"), vpnProfile("lab", "10.9.9.1:22")]}
      />,
    );

    expect(screen.getByRole("option", { name: "tohoku" })).toBeEnabled();
    expect(screen.getByRole("option", { name: "lab (reaches 10.9.9.1:22, not this connection)" })).toBeDisabled();
  });

  it("explains a mismatch when the connection already points at a profile for another host", () => {
    const detail = build();
    detail.metadata = { ...detail.metadata, vpn: "lab" };

    render(<HostInspector detail={detail} onMetadata={vi.fn()} vpnProfiles={[vpnProfile("lab", "10.9.9.1:22")]} />);

    expect(screen.getByLabelText("VPN route")).toHaveValue("lab");
    expect(screen.getByRole("alert")).toHaveTextContent("This connection goes to bastion:22, but lab only reaches 10.9.9.1:22.");
  });

  it("compares against the HostName and Port the connection sets", () => {
    const detail = build();
    detail.form.fields = [
      { keyword: "HostName", values: ["10.9.9.1"], line: 2, category: "basic", editable: true },
      { keyword: "Port", values: ["2222"], line: 3, category: "basic", editable: true },
    ];

    render(
      <HostInspector
        detail={detail}
        onMetadata={vi.fn()}
        vpnProfiles={[vpnProfile("lab", "10.9.9.1:22"), vpnProfile("lab-alt", "10.9.9.1:2222")]}
      />,
    );

    expect(screen.getByRole("option", { name: "lab-alt" })).toBeEnabled();
    expect(screen.getByRole("option", { name: /^lab \(reaches/ })).toBeDisabled();
  });

  it("clears a colour rather than leaving the picker's fallback as a real value", async () => {
    const onMetadata = vi.fn();
    const user = userEvent.setup();
    const detail = build();
    detail.metadata = { ...detail.metadata, colour: "#f97316" };
    render(<HostInspector detail={detail} onMetadata={onMetadata} />);

    await user.click(screen.getByRole("button", { name: "Clear colour" }));

    expect(onMetadata).toHaveBeenLastCalledWith(expect.objectContaining({ colour: "" }));
  });

  it("offers no clear button when there is no colour to clear", () => {
    render(<HostInspector detail={build()} onMetadata={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "Clear colour" })).not.toBeInTheDocument();
  });

  it("does not offer the group, the comment or the rename", () => {
    render(<HostInspector detail={build()} onMetadata={vi.fn()} />);

    expect(screen.queryByLabelText("Primary group")).toBeNull();
    expect(screen.queryByLabelText("Comment")).toBeNull();
    expect(screen.queryByLabelText("Rename alias")).toBeNull();
  });

  it("lists the notices this connection has", () => {
    const detail = build();
    detail.form.notices = [{ code: "duplicate_alias", path: "config", line: 41 }];
    render(<HostInspector detail={detail} onMetadata={vi.fn()} />);

    expect(screen.getByText(/config:41/)).toBeInTheDocument();
  });

  it("lists only the values that came from elsewhere", () => {
    const detail = build();
    detail.effective.entries = [
      { keyword: "Port", values: ["22"], source: { path: "groups.sshc.conf", line: 3 } },
      { keyword: "User", values: ["aida"], source: { path: "connections/work/bastion.conf", line: 2 } },
    ];
    render(<HostInspector detail={detail} onMetadata={vi.fn()} />);

    expect(screen.getByText(/Port 22/)).toBeInTheDocument();
    expect(screen.queryByText(/User aida/)).toBeNull();
  });
});

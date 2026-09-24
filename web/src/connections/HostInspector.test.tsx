import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { HostInspector } from "./HostInspector";
import type { HostDetail } from "../api/config";
import type { VPNProfile } from "../api/vpn";

// vpnProfile は、WireGuard のプロファイルである。dns は VPN 内の DNS サーバーである。
function vpnProfile(name: string, dns: string[] = []): VPNProfile {
  return {
    name,
    backend: "wireguard",
    ...(dns.length === 0 ? {} : { dns }),
    wireguard: { server: "vpn.example.jp:51820", peerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=", address: "10.9.9.2/32" },
  };
}

const profiles = [vpnProfile("tohoku", ["10.9.9.53"]), vpnProfile("office")];

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

// saveButton は、下書きに変更があるときだけ現れる保存ボタンである。
function saveButton() {
  return screen.queryByRole("button", { name: "Save sshc-only settings" });
}

describe("HostInspector", () => {
  it("changes the OS override and can return to automatic detection", async () => {
    const user = userEvent.setup();
    const detail = build();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const { rerender } = render(<HostInspector detail={detail} onSave={onSave} />);
    await user.selectOptions(screen.getByRole("combobox", { name: "Operating system icon" }), "ubuntu");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith({ ...detail.metadata, os: "ubuntu" });

    const saved = { ...detail, metadata: { ...detail.metadata, os: "ubuntu" as const } };
    rerender(<HostInspector detail={saved} onSave={onSave} />);
    await user.selectOptions(screen.getByRole("combobox", { name: "Operating system icon" }), "");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith({ ...saved.metadata, os: "" });
  });

  it("edits the display settings that live only in metadata", () => {
    render(<HostInspector detail={build()} onSave={vi.fn()} />);

    expect(screen.queryByText(/saved immediately/)).toBeNull();
    expect(screen.getByLabelText(/^Tags/)).toBeInTheDocument();
    expect(screen.getByLabelText(/^Display order/)).toBeInTheDocument();
    expect(screen.getByLabelText("Colour")).toBeInTheDocument();
  });

  it("offers neither save nor discard until something changes", () => {
    render(<HostInspector detail={build()} onSave={vi.fn()} />);

    expect(saveButton()).toBeNull();
    expect(screen.queryByRole("button", { name: "Discard changes" })).toBeNull();
  });

  it("keeps a change as a draft until Save is pressed", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const onDirtyChange = vi.fn();
    render(<HostInspector detail={build()} onSave={onSave} onDirtyChange={onDirtyChange} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");

    expect(onSave).not.toHaveBeenCalled();
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);
    expect(saveButton()).toBeEnabled();
    expect(screen.getByRole("button", { name: "Discard changes" })).toBeEnabled();
  });

  it("saves every change in the draft with one save", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<HostInspector detail={build()} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    await user.selectOptions(screen.getByLabelText("OSC 52 clipboard"), "deny");
    await user.click(saveButton()!);

    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ encoding: "shift_jis", osc52: "deny" }));
  });

  it("discards the draft back to the saved values", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const onDirtyChange = vi.fn();
    render(<HostInspector detail={build()} onSave={onSave} onDirtyChange={onDirtyChange} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    await user.click(screen.getByRole("button", { name: "Discard changes" }));

    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("");
    expect(saveButton()).toBeNull();
    expect(onDirtyChange).toHaveBeenLastCalledWith(false);
    expect(onSave).not.toHaveBeenCalled();
  });

  it("hands the page a way to discard the draft", async () => {
    const user = userEvent.setup();
    let discard: (() => void) | null = null;
    render(<HostInspector detail={build()} onSave={vi.fn()} onDiscardReady={(next) => { discard = next; }} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    act(() => (discard as unknown as () => void)());

    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("");
    expect(saveButton()).toBeNull();
  });

  it("treats a change that is undone by hand as no change", async () => {
    const user = userEvent.setup();
    render(<HostInspector detail={build()} onSave={vi.fn()} />);

    await user.selectOptions(screen.getByRole("combobox", { name: "Operating system icon" }), "ubuntu");
    await user.selectOptions(screen.getByRole("combobox", { name: "Operating system icon" }), "");

    expect(saveButton()).toBeNull();
  });

  it("shows that it is saving and locks the draft until the save finishes", async () => {
    const user = userEvent.setup();
    let finish: () => void = () => undefined;
    const onSave = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
    render(<HostInspector detail={build()} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    await user.click(saveButton()!);

    expect(screen.getByRole("button", { name: "Saving…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Discard changes" })).toBeDisabled();
    expect(screen.getByLabelText("Remote text encoding")).toBeDisabled();
    await act(async () => finish());
    expect(screen.getByLabelText("Remote text encoding")).toBeEnabled();
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("says the save failed and keeps the draft to try again", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockRejectedValueOnce(new Error("metadata_not_saved")).mockResolvedValueOnce(undefined);
    render(<HostInspector detail={build()} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    await user.click(saveButton()!);

    expect(await screen.findByRole("alert")).toHaveTextContent("The sshc-only settings could not be saved.");
    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("shift_jis");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("starts over from the saved values once the saved metadata changes", async () => {
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(<HostInspector detail={detail} onSave={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    rerender(<HostInspector detail={{ ...detail, metadata: { ...detail.metadata, encoding: "euc-jp" } }} onSave={vi.fn()} />);

    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("euc-jp");
    expect(saveButton()).toBeNull();
  });

  it("keeps the draft when the same saved metadata is loaded again", async () => {
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(<HostInspector detail={detail} onSave={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    rerender(<HostInspector detail={{ ...detail, metadata: { ...detail.metadata } }} onSave={vi.fn()} />);

    expect(screen.getByLabelText("Remote text encoding")).toHaveValue("shift_jis");
    expect(saveButton()).toBeEnabled();
  });

  it("cannot be edited while another tab has a draft", () => {
    render(<HostInspector detail={build()} onSave={vi.fn()} disabled />);

    expect(screen.getByLabelText("Remote text encoding")).toBeDisabled();
    expect(screen.getByLabelText(/^Tags/)).toBeDisabled();
  });

  it("keeps the commas typed in the tags and saves the tags they separate", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<HostInspector detail={build()} onSave={onSave} />);

    await user.type(screen.getByLabelText(/^Tags/), "web, db,");
    expect(screen.getByLabelText(/^Tags/)).toHaveValue("web, db,");
    await user.click(saveButton()!);

    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ tags: ["web", "db"] }));
  });

  it("edits the display order the metadata schema has always carried", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<HostInspector detail={build()} onSave={onSave} />);

    await user.type(screen.getByLabelText(/^Display order/), "7");
    await user.click(saveButton()!);

    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ order: 7 }));
  });

  it("saves the remote encoding per connection and can return to UTF-8", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(<HostInspector detail={detail} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "shift_jis");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ encoding: "shift_jis" }));

    rerender(<HostInspector detail={{ ...detail, metadata: { ...detail.metadata, encoding: "shift_jis" } }} onSave={onSave} />);
    await user.selectOptions(screen.getByLabelText("Remote text encoding"), "");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith(expect.not.objectContaining({ encoding: expect.anything() }));
  });

  it("stores an OSC 52 override per SSH connection and can inherit again", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(<HostInspector detail={detail} onSave={onSave} />);

    await user.selectOptions(screen.getByLabelText("OSC 52 clipboard"), "deny");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ osc52: "deny" }));

    rerender(<HostInspector detail={{ ...detail, metadata: { ...detail.metadata, osc52: "deny" } }} onSave={onSave} />);
    await user.selectOptions(screen.getByLabelText("OSC 52 clipboard"), "");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith(expect.not.objectContaining({ osc52: expect.anything() }));
  });

  it("routes this connection through a VPN profile and can stop routing it", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    const detail = build();
    const { rerender } = render(
      <HostInspector detail={detail} onSave={onSave} vpnProfiles={profiles} />,
    );

    await user.selectOptions(screen.getByLabelText("VPN profile"), "tohoku");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ vpn: "tohoku" }));

    rerender(
      <HostInspector detail={{ ...detail, metadata: { ...detail.metadata, vpn: "tohoku" } }} onSave={onSave} vpnProfiles={profiles} />,
    );
    await user.selectOptions(screen.getByLabelText("VPN profile"), "");
    await user.click(saveButton()!);
    expect(onSave).toHaveBeenLastCalledWith(expect.not.objectContaining({ vpn: expect.anything() }));
  });

  it("still names a VPN profile that no longer exists, instead of showing no route", () => {
    const detail = build();
    detail.metadata = { ...detail.metadata, vpn: "retired" };

    render(<HostInspector detail={detail} onSave={vi.fn()} vpnProfiles={[vpnProfile("tohoku")]} />);

    expect(screen.getByLabelText("VPN profile")).toHaveValue("retired");
    expect(screen.getByRole("option", { name: "retired (profile is gone)" })).toBeInTheDocument();
  });

  it("offers every VPN profile, whatever host the connection goes to", () => {
    render(<HostInspector detail={build()} onSave={vi.fn()} vpnProfiles={profiles} />);

    expect(screen.getByRole("option", { name: "tohoku" })).toBeEnabled();
    expect(screen.getByRole("option", { name: "office" })).toBeEnabled();
  });

  // build() の接続は HostName を持たないので、接続先は alias の bastion（ホスト名）になる。
  it("notes that a host name needs a DNS server inside the VPN when the chosen profile has none", () => {
    const detail = build();
    detail.metadata = { ...detail.metadata, vpn: "office" };

    render(<HostInspector detail={detail} onSave={vi.fn()} vpnProfiles={profiles} />);

    expect(screen.getByLabelText("VPN profile")).toHaveAccessibleDescription(
      "This HostName is a host name, so office needs a DNS server inside the VPN. Edit office on the VPN screen to add one, or set HostName to an IPv4 address.",
    );
  });

  it("says nothing more when the chosen profile has a DNS server inside the VPN", () => {
    const detail = build();
    detail.metadata = { ...detail.metadata, vpn: "tohoku" };

    render(<HostInspector detail={detail} onSave={vi.fn()} vpnProfiles={profiles} />);

    expect(screen.getByLabelText("VPN profile")).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("says nothing more when HostName is an IPv4 address", () => {
    const detail = build();
    detail.form.fields = [{ keyword: "HostName", values: ["10.9.9.1"], line: 2, category: "basic", editable: true }];
    detail.metadata = { ...detail.metadata, vpn: "office" };

    render(<HostInspector detail={detail} onSave={vi.fn()} vpnProfiles={profiles} />);

    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("does not guess when HostName is set in more than one way", () => {
    const detail = build();
    detail.form.fields = [
      { keyword: "HostName", values: ["lab.example.jp"], line: 2, category: "basic", editable: true },
      { keyword: "HostName", values: ["10.9.9.1"], line: 3, category: "basic", editable: true },
    ];
    detail.metadata = { ...detail.metadata, vpn: "office" };

    render(<HostInspector detail={detail} onSave={vi.fn()} vpnProfiles={profiles} />);

    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("clears a colour rather than leaving the picker's fallback as a real value", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    const detail = build();
    detail.metadata = { ...detail.metadata, colour: "#f97316" };
    render(<HostInspector detail={detail} onSave={onSave} />);

    await user.click(screen.getByRole("button", { name: "Clear colour" }));
    await user.click(saveButton()!);

    expect(onSave).toHaveBeenLastCalledWith(expect.objectContaining({ colour: "" }));
  });

  it("offers no clear button when there is no colour to clear", () => {
    render(<HostInspector detail={build()} onSave={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "Clear colour" })).not.toBeInTheDocument();
  });

  it("does not offer the group, the comment or the rename", () => {
    render(<HostInspector detail={build()} onSave={vi.fn()} />);

    expect(screen.queryByLabelText("Primary group")).toBeNull();
    expect(screen.queryByLabelText("Comment")).toBeNull();
    expect(screen.queryByLabelText("Rename alias")).toBeNull();
  });

  it("lists the notices this connection has", () => {
    const detail = build();
    detail.form.notices = [{ code: "duplicate_alias", path: "config", line: 41 }];
    render(<HostInspector detail={detail} onSave={vi.fn()} />);

    expect(screen.getByText(/config:41/)).toBeInTheDocument();
  });

  it("lists only the values that came from elsewhere", () => {
    const detail = build();
    detail.effective.entries = [
      { keyword: "Port", values: ["22"], source: { path: "groups.sshc.conf", line: 3 } },
      { keyword: "User", values: ["aida"], source: { path: "connections/work/bastion.conf", line: 2 } },
    ];
    render(<HostInspector detail={detail} onSave={vi.fn()} />);

    expect(screen.getByText(/Port 22/)).toBeInTheDocument();
    expect(screen.queryByText(/User aida/)).toBeNull();
  });
});

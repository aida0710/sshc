import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { VPNProfile } from "../api/vpn";
import { VPNProfileForm } from "./VPNProfileForm";

// OpenVPN を選んだときのフォーム。設定ファイルを読み込み、remote のサーバーを設定として、
// 設定ファイルをシークレットとして送る。

const config = [
  "client",
  "dev tun",
  "remote vpn.example.jp 1194",
  "remote backup.example.jp 443",
  "auth-user-pass",
  "<ca>",
  "-----BEGIN CERTIFICATE-----",
  "-----END CERTIFICATE-----",
  "</ca>",
  "",
].join("\n");

const savedProfile: VPNProfile = {
  name: "provider",
  backend: "openvpn",
  openvpn: { servers: ["vpn.example.jp"], username: "fixture" },
};

function renderForm(editing?: VPNProfile) {
  const onSave = vi.fn().mockResolvedValue({ saved: true });
  render(<VPNProfileForm busy={false} onSave={onSave} {...(editing === undefined ? {} : { editing, onCancel: () => {} })} />);
  return onSave;
}

// pasteConfig は、設定ファイルの欄へ貼り付ける。user.type は < と { を特別に読むので、値を直に入れる。
function pasteConfig(form: HTMLElement, text: string) {
  fireEvent.change(within(form).getByLabelText("Configuration file (.ovpn)"), { target: { value: text } });
}

describe("VPNProfileForm with OpenVPN", () => {
  it("reads the servers out of the file and sends the file as a secret", async () => {
    const user = userEvent.setup();
    const onSave = renderForm();
    const form = screen.getByRole("region", { name: "Add a VPN profile" });

    await user.type(within(form).getByLabelText("Name"), "provider");
    await user.selectOptions(within(form).getByLabelText("Type"), "openvpn");
    expect(within(form).queryByLabelText("VPN server")).toBeNull();
    pasteConfig(form, config);
    expect(within(form).getByText("VPN servers (remote): vpn.example.jp, backup.example.jp")).toBeVisible();
    expect(within(form).getByText("This file asks for a username and a password (auth-user-pass).")).toBeVisible();
    await user.type(within(form).getByLabelText("VPN username"), "fixture");
    await user.type(within(form).getByLabelText("VPN password"), "fixture-password");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(onSave).toHaveBeenCalledWith(
      { name: "provider", backend: "openvpn", openvpn: { servers: ["vpn.example.jp", "backup.example.jp"], username: "fixture" } },
      { openvpnConfig: config, openvpnPassword: "fixture-password" },
    );
  });

  it("names the refused directive and its line as soon as the file is pasted", async () => {
    const user = userEvent.setup();
    const onSave = renderForm();
    const form = screen.getByRole("region", { name: "Add a VPN profile" });

    await user.type(within(form).getByLabelText("Name"), "provider");
    await user.selectOptions(within(form).getByLabelText("Type"), "openvpn");
    pasteConfig(form, "client\nremote vpn.example.jp\nca /etc/openvpn/ca.crt\n");

    expect(within(form).getByRole("alert")).toHaveTextContent(
      'Line 3: "ca" names a file, which the container cannot read. Embed its contents as <ca>…</ca>.',
    );
    await user.click(within(form).getByRole("button", { name: "Save" }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it("keeps the stored file and password when they are left blank while editing", async () => {
    const user = userEvent.setup();
    const onSave = renderForm(savedProfile);
    const form = screen.getByRole("region", { name: "Edit provider" });

    expect(within(form).getByText("VPN servers (remote): vpn.example.jp")).toBeVisible();
    expect(within(form).getByLabelText("VPN username")).toHaveValue("fixture");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(savedProfile, {}));
  });

  it("fills the stored file and password when editing opens, and shows the password only on request", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue({ saved: true });
    const revealSecrets = vi.fn().mockResolvedValue({ openvpnConfig: config, openvpnPassword: "the stored password" });
    render(
      <VPNProfileForm busy={false} editing={savedProfile} revealSecrets={revealSecrets} onSave={onSave} onCancel={() => {}} />,
    );
    const form = screen.getByRole("region", { name: "Edit provider" });

    await waitFor(() => expect(within(form).getByLabelText("Configuration file (.ovpn)")).toHaveValue(config));
    const password = within(form).getByLabelText("VPN password");
    expect(password).toHaveValue("the stored password");
    expect(password).toHaveAttribute("type", "password");
    await user.click(within(form).getByRole("button", { name: "Show VPN password" }));
    expect(password).toHaveAttribute("type", "text");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(
      { ...savedProfile, openvpn: { servers: ["vpn.example.jp", "backup.example.jp"], username: "fixture" } },
      { openvpnConfig: config, openvpnPassword: "the stored password" },
    ));
  });
});

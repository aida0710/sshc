import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { VPNProfile, VPNSecrets } from "../api/vpn";
import { VPNProfileForm } from "./VPNProfileForm";

// WireGuard を選んだときのフォーム。設定ファイルは鍵を含めて全体がシークレットで、編集を開くと
// 保存済みの設定ファイルを取り出してそのまま見せる。

const privateKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=";
const publicKey = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=";
const presharedKey = "dDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDA=";

// storedConfig は、Vault にある設定ファイルである。
const storedConfig = [
  "[Interface]",
  `PrivateKey = ${privateKey}`,
  "Address = 10.64.1.2/32",
  "DNS = 10.64.0.1",
  "",
  "[Peer]",
  `PublicKey = ${publicKey}`,
  `PresharedKey = ${presharedKey}`,
  "Endpoint = vpn.example.jp:51820",
  "AllowedIPs = 10.64.0.0/16",
  "",
].join("\n");

const savedProfile: VPNProfile = {
  name: "provider",
  backend: "wireguard",
  dns: ["10.64.0.1"],
  wireguard: { servers: ["vpn.example.jp"] },
};

function renderEditing(revealSecrets: (name: string) => Promise<VPNSecrets>) {
  const onSave = vi.fn().mockResolvedValue({ saved: true });
  const onCancel = vi.fn();
  render(
    <VPNProfileForm busy={false} editing={savedProfile} revealSecrets={revealSecrets} onSave={onSave} onCancel={onCancel} />,
  );
  return { onSave, onCancel, form: screen.getByRole("region", { name: "Edit provider" }) };
}

function configField(form: HTMLElement) {
  return within(form).getByRole("textbox", { name: "Configuration file" });
}

function editConfig(form: HTMLElement, text: string) {
  fireEvent.change(configField(form), { target: { value: text } });
}

describe("VPNProfileForm with WireGuard", () => {
  it("shows the whole stored configuration file, keys included, and saves it as it is", async () => {
    const user = userEvent.setup();
    const revealSecrets = vi.fn().mockResolvedValue({ wireguardConfig: storedConfig });
    const { onSave, form } = renderEditing(revealSecrets);

    await waitFor(() => expect(configField(form)).toHaveValue(storedConfig));
    expect(revealSecrets).toHaveBeenCalledWith("provider");
    expect(within(form).getByText("VPN servers (Endpoint): vpn.example.jp")).toBeVisible();
    expect(within(form).getByText("DNS inside the VPN (DNS): 10.64.0.1")).toBeVisible();
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(savedProfile, { wireguardConfig: storedConfig }));
  });

  it("puts the keys of a profile saved before configuration files back into the text it shows", async () => {
    // v0.40.0 までの項目の形のプロファイルは、engine が項目と秘密鍵から設定ファイルを組み立てて返す。
    const builtConfig = [
      "[Interface]", `PrivateKey = ${privateKey}`, "Address = 10.9.9.2/32", "",
      "[Peer]", `PublicKey = ${publicKey}`, "Endpoint = vpn.example.jp:51820", "AllowedIPs = 0.0.0.0/0",
      "PersistentKeepalive = 25", "",
    ].join("\n");
    const { form } = renderEditing(vi.fn().mockResolvedValue({ wireguardConfig: builtConfig }));

    await waitFor(() => expect(configField(form)).toHaveValue(builtConfig));
    const shown = (configField(form) as HTMLTextAreaElement).value;
    expect(shown).toContain(`PrivateKey = ${privateKey}`);
    expect(shown).not.toContain("(saved)");
  });

  it("reads the servers and DNS again from an edited file", async () => {
    const user = userEvent.setup();
    const { onSave, form } = renderEditing(vi.fn().mockResolvedValue({ wireguardConfig: storedConfig }));
    await waitFor(() => expect(configField(form)).toHaveValue(storedConfig));

    const edited = storedConfig.replace("vpn.example.jp:51820", "vpn2.example.jp:51820").replace("DNS = 10.64.0.1\n", "");
    editConfig(form, edited);
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(
      { name: "provider", backend: "wireguard", wireguard: { servers: ["vpn2.example.jp"] } },
      { wireguardConfig: edited },
    ));
  });

  it("forgets the configuration file it took out when editing is cancelled", async () => {
    const user = userEvent.setup();
    const { onCancel, form } = renderEditing(vi.fn().mockResolvedValue({ wireguardConfig: storedConfig }));
    await waitFor(() => expect(configField(form)).toHaveValue(storedConfig));

    await user.click(within(form).getByRole("button", { name: "Cancel" }));

    expect(onCancel).toHaveBeenCalled();
    expect(configField(form)).toHaveValue("");
  });

  it("keeps the stored file when it could not be taken out and the field is left blank", async () => {
    const user = userEvent.setup();
    const { onSave, form } = renderEditing(vi.fn().mockRejectedValue(
      new ApiError("vault_locked", 409, { code: "vault_locked", message: "request rejected" }),
    ));

    expect(await within(form).findByText(/^The stored secrets could not be taken out of the vault\./)).toBeVisible();
    expect(configField(form)).toHaveValue("");
    expect(within(form).getByText(/^Leave blank to keep the stored file\./)).toBeVisible();
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(savedProfile, {}));
  });

  it("names a directive that runs a command as soon as it is written", async () => {
    const { form } = renderEditing(vi.fn().mockResolvedValue({ wireguardConfig: storedConfig }));
    await waitFor(() => expect(configField(form)).toHaveValue(storedConfig));

    editConfig(form, storedConfig.replace("DNS = 10.64.0.1", "PostUp = iptables -A FORWARD -j ACCEPT"));

    expect(within(form).getByRole("alert")).toHaveTextContent(
      'Line 4: "PostUp" runs a command or loads a program, so it cannot be used.',
    );
  });
});

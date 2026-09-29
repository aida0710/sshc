import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import profileCases from "../../../internal/vpn/testdata/profile-cases.json";
import { VPNProfileForm } from "./VPNProfileForm";

// caCertificate は、共有の表で engine が受け付ける CA の証明書である。
const caCertificate = profileCases.cases
  .flatMap((test) => ("ikev2" in test.profile ? [test.profile.ikev2?.caCertificate ?? ""] : []))
  .find((certificate) => certificate !== "") ?? "";

// renderForm は、作成のフォームを出し、IKEv2/IPsec を選んで名前とサーバーを入れる。
async function renderForm() {
  const user = userEvent.setup();
  const onSave = vi.fn().mockResolvedValue({ saved: true });
  render(<VPNProfileForm busy={false} onSave={onSave} />);
  await user.selectOptions(screen.getByLabelText("Type"), "ikev2");
  await user.type(screen.getByLabelText("Name"), "office");
  await user.type(screen.getByLabelText("VPN server"), "vpn.example.jp");
  return { user, onSave };
}

describe("IKEv2ProfileFields", () => {
  it("sends a username, a password and the CA certificate for EAP", async () => {
    const { user, onSave } = await renderForm();

    await user.type(screen.getByLabelText("VPN username"), "tester");
    await user.type(screen.getByLabelText("VPN password"), "a password");
    await user.type(screen.getByLabelText("Server ID (remote ID)"), "vpn.example.jp");
    await user.click(screen.getByLabelText("CA certificate (PEM)"));
    await user.paste(caCertificate);
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onSave).toHaveBeenCalledWith(
      {
        name: "office",
        backend: "ikev2",
        ikev2: {
          server: "vpn.example.jp",
          authentication: "eap-mschapv2",
          identity: "tester",
          serverIdentity: "vpn.example.jp",
          caCertificate,
        },
      },
      { ikev2Password: "a password" },
    );
  });

  it("asks for a local ID and a pre-shared key, and never sends a CA certificate, for PSK", async () => {
    const { user, onSave } = await renderForm();

    await user.click(screen.getByLabelText("CA certificate (PEM)"));
    await user.paste(caCertificate);
    await user.selectOptions(screen.getByLabelText("Authentication"), "psk");
    expect(screen.queryByLabelText("CA certificate (PEM)")).toBeNull();
    expect(screen.queryByLabelText("VPN password")).toBeNull();
    await user.type(screen.getByLabelText("Local ID"), "branch@example.jp");
    await user.type(screen.getByLabelText("IPsec pre-shared key"), "a key");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onSave).toHaveBeenCalledWith(
      {
        name: "office",
        backend: "ikev2",
        ikev2: { server: "vpn.example.jp", authentication: "psk", identity: "branch@example.jp" },
      },
      { ikev2Psk: "a key" },
    );
  });

  it("says a pasted private key is not a CA certificate, before sending", async () => {
    const { user, onSave } = await renderForm();

    await user.type(screen.getByLabelText("VPN username"), "tester");
    await user.type(screen.getByLabelText("VPN password"), "a password");
    await user.click(screen.getByLabelText("CA certificate (PEM)"));
    await user.paste("-----BEGIN PRIVATE KEY-----\nMAMCAQE=\n-----END PRIVATE KEY-----\n");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByLabelText("CA certificate (PEM)")).toHaveAccessibleDescription("This is not written in a form this field accepts.");
  });
});

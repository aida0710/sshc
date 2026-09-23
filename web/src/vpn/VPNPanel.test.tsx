import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { VPNApi, VPNOverview } from "../api/vpn";
import { VPNPanel } from "./VPNPanel";

function overview(overrides: Partial<VPNOverview> = {}): VPNOverview {
  return {
    available: true,
    profiles: [
      {
        profile: { name: "tohoku", backend: "l2tp_ipsec", target: "10.9.9.1:22" },
        running: true,
        relaySocket: "/home/tester/.ssh/sshc/vpn/tohoku/relay.sock",
        connections: ["lab"],
      },
    ],
    ...overrides,
  };
}

function buildApi(overrides: Partial<VPNApi> = {}): VPNApi {
  return {
    vpnOverview: vi.fn().mockResolvedValue(overview()),
    saveVPNProfile: vi.fn().mockResolvedValue(overview()),
    removeVPNProfile: vi.fn().mockResolvedValue(overview({ profiles: [] })),
    startVPNSession: vi.fn().mockResolvedValue(overview()),
    stopVPNSession: vi.fn().mockResolvedValue(overview()),
    setConnectionVPN: vi.fn().mockResolvedValue(overview()),
    ...overrides,
  };
}

describe("VPNPanel", () => {
  it("shows each route with its state and the connections that use it", async () => {
    render(<VPNPanel api={buildApi()} aliases={["lab", "edge"]} />);

    const route = await screen.findByRole("article", { name: "tohoku" });
    expect(within(route).getByText(/l2tp_ipsec/)).toBeVisible();
    expect(within(route).getByText(/10\.9\.9\.1:22/)).toBeVisible();
    expect(within(route).getByText("lab")).toBeVisible();
  });

  it("says why this machine cannot open routes instead of hiding the screen", async () => {
    const api = buildApi({
      vpnOverview: vi.fn().mockResolvedValue(
        overview({ available: false, detail: "docker is not available", profiles: [] }),
      ),
    });

    render(<VPNPanel api={api} />);

    expect(await screen.findByText(/docker is not available/)).toBeVisible();
  });

  it("sends the secrets with the profile and never shows them again", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.type(within(form).getByLabelText("Name"), "lab");
    await user.type(within(form).getByLabelText("Target inside the VPN"), "10.9.9.1:22");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp:51820");
    await user.type(within(form).getByLabelText("Peer public key"), "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=");
    await user.type(within(form).getByLabelText("Private key"), "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(
      {
        name: "lab",
        backend: "wireguard",
        target: "10.9.9.1:22",
        wireguard: {
          server: "vpn.example.jp:51820",
          peerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=",
          address: "10.0.0.2/32",
        },
      },
      { wireguardPrivateKey: "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=" },
    );
    expect(within(form).getByLabelText("Private key")).toHaveValue("");
  });

  it("routes a chosen connection through the profile", async () => {
    const user = userEvent.setup();
    const setConnectionVPN = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ setConnectionVPN })} aliases={["lab", "edge"]} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.selectOptions(within(route).getByLabelText("Choose a connection"), "edge");
    await user.click(within(route).getByRole("button", { name: "Route through this VPN" }));

    expect(setConnectionVPN).toHaveBeenCalledWith("edge", "tohoku");
  });

  it("stops routing a connection when its binding is removed", async () => {
    const user = userEvent.setup();
    const setConnectionVPN = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ setConnectionVPN })} aliases={["lab"]} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Stop routing lab through tohoku" }));

    expect(setConnectionVPN).toHaveBeenCalledWith("lab", "");
  });
});

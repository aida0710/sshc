import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
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
        tunnel: {
          backend: "l2tp_ipsec",
          interface: "ppp0",
          address: "10.20.30.40",
          since: "2026-09-23T01:02:03Z",
        },
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
    renameVPNProfile: vi.fn().mockResolvedValue(overview()),
    vpnLogs: vi.fn().mockResolvedValue({ lines: "starting IPsec\n" }),
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
    expect(saveVPNProfile.mock.calls[0]?.[0]).not.toHaveProperty("dns");
  });

  it("sends the VPN's own DNS servers with a profile whose target is a name", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.type(within(form).getByLabelText("Name"), "lab");
    await user.type(within(form).getByLabelText("Target inside the VPN"), "lab.example.jp:22");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp:51820");
    await user.type(within(form).getByLabelText("DNS inside the VPN"), "10.9.9.53, 10.9.9.54");
    await user.type(within(form).getByLabelText("Peer public key"), "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=");
    await user.type(within(form).getByLabelText("Private key"), "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({ target: "lab.example.jp:22", dns: ["10.9.9.53", "10.9.9.54"] }),
      expect.anything(),
    );
  });

  it("waits for approval without sending a second answer when the device asks nothing", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("Target inside the VPN"), "10.9.9.1:22");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "approve");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    const [profile] = saveVPNProfile.mock.calls[0] ?? [];
    expect(profile.openconnect).toEqual(
      expect.objectContaining({ secondFactor: "approve" }),
    );
    expect(profile.openconnect).not.toHaveProperty("approvalWord");
  });

  it("sends the word the device asks for when one is given", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("Target inside the VPN"), "10.9.9.1:22");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "approve");
    await user.type(within(form).getByLabelText("Word to send as the second answer"), "push");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({
        openconnect: expect.objectContaining({ secondFactor: "approve", approvalWord: "push" }),
      }),
      { openconnectPassword: "a password" },
    );
  });

  it("carries the TOTP seed only when the second factor uses one", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("Target inside the VPN"), "10.9.9.1:22");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "totp");
    // 種を入れるまでは保存できない。
    expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled();
    await user.type(within(form).getByLabelText("Second factor TOTP seed"), "GEZDGNBVGY3TQOJQ");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({ openconnect: expect.objectContaining({ secondFactor: "totp" }) }),
      { openconnectPassword: "a password", openconnectTotpSecret: "GEZDGNBVGY3TQOJQ" },
    );
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

  it("says what it is waiting for while a route is being opened, and stops saying it once it is up", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const starting = overview({
        profiles: [{
          profile: { name: "tohoku", backend: "l2tp_ipsec", target: "10.9.9.1:22" },
          running: true,
          relaySocket: "",
          connections: [],
          phase: "image",
        }],
      });
      const vpnOverview = vi.fn().mockResolvedValueOnce(starting).mockResolvedValue(overview());
      render(<VPNPanel api={buildApi({ vpnOverview })} />);

      expect(await screen.findByText(/building the image/)).toBeVisible();

      await act(async () => {
        vi.advanceTimersByTime(2000);
        await Promise.resolve();
      });

      await waitFor(() => expect(screen.getByText(/route open/)).toBeVisible());
      expect(screen.queryByText(/building the image/)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows what the tunnel inside the container is carrying", async () => {
    render(<VPNPanel api={buildApi()} />);

    const route = await screen.findByRole("article", { name: "tohoku" });
    expect(within(route).getByText("ppp0")).toBeVisible();
    expect(within(route).getByText("10.20.30.40")).toBeVisible();
  });

  it("shows the container's output without sending anyone to docker", async () => {
    const user = userEvent.setup();
    const vpnLogs = vi.fn().mockResolvedValue({ lines: "starting IPsec\n" });
    render(<VPNPanel api={buildApi({ vpnLogs })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Logs" }));

    expect(vpnLogs).toHaveBeenCalledWith("tohoku");
    expect(await screen.findByText(/starting IPsec/)).toBeVisible();
  });

  it("explains why the logs could not be read instead of showing nothing", async () => {
    const user = userEvent.setup();
    const vpnLogs = vi.fn().mockRejectedValue(new ApiError("vpn_docker_missing", 409, null));
    render(<VPNPanel api={buildApi({ vpnLogs })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Logs" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/Docker/);
  });

  it("renames a route to the name that was typed", async () => {
    const user = userEvent.setup();
    const renameVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ renameVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Rename" }));
    const dialog = screen.getByRole("dialog");
    await user.clear(within(dialog).getByLabelText("New name"));
    await user.type(within(dialog).getByLabelText("New name"), "tains");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));

    expect(renameVPNProfile).toHaveBeenCalledWith("tohoku", "tains");
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

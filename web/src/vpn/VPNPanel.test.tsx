import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { VPNApi, VPNOverview, VPNProfile } from "../api/vpn";
import { VPNPanel } from "./VPNPanel";
import { routeProgressIntervalMs } from "./vpnPhases";

// tohokuProfile は、L2TP/IPsec のプロファイルである。接続先は持たない。
const tohokuProfile: VPNProfile = {
  name: "tohoku",
  backend: "l2tp_ipsec",
  dns: ["10.9.9.53"],
  l2tp: { server: "vpn.example.jp", username: "tester" },
};

function overview(overrides: Partial<VPNOverview> = {}): VPNOverview {
  return {
    available: true,
    profiles: [
      {
        profile: tohokuProfile,
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

const peerPublicKey = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=";
const privateKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=";

// startingOverview は、tohoku の経路を用意している最中の一覧である。
function startingOverview(): VPNOverview {
  return overview({
    profiles: [{
      profile: tohokuProfile,
      running: true,
      relaySocket: "",
      connections: [],
      phase: "image",
    }],
  });
}

// stoppedOverview は、tohoku が止まっている一覧である。
function stoppedOverview(): VPNOverview {
  return overview({
    profiles: [{
      profile: tohokuProfile,
      running: false,
      relaySocket: "",
      connections: [],
    }],
  });
}

// fillWireGuardProfile は、作成フォームへ WireGuard のプロファイルをひとつ入れる。
async function fillWireGuardProfile(user: ReturnType<typeof userEvent.setup>, form: HTMLElement) {
  await user.type(within(form).getByLabelText("Name"), "lab");
  await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp:51820");
  await user.type(within(form).getByLabelText("Peer public key"), peerPublicKey);
  await user.type(within(form).getByLabelText("Private key"), privateKey);
}

function buildApi(overrides: Partial<VPNApi> = {}): VPNApi {
  return {
    vpnOverview: vi.fn().mockResolvedValue(overview()),
    createVPNProfile: vi.fn().mockResolvedValue(overview()),
    saveVPNProfile: vi.fn().mockResolvedValue(overview()),
    removeVPNProfile: vi.fn().mockResolvedValue(overview({ profiles: [] })),
    renameVPNProfile: vi.fn().mockResolvedValue(overview()),
    vpnLogs: vi.fn().mockResolvedValue({ lines: "starting IPsec\n" }),
    startVPNSession: vi.fn().mockResolvedValue(overview()),
    stopVPNSession: vi.fn().mockResolvedValue(overview()),
    ...overrides,
  };
}

describe("VPNPanel", () => {
  it("shows each profile with its type, server, state and the connections that use it", async () => {
    render(<VPNPanel api={buildApi()} />);

    const route = await screen.findByRole("article", { name: "tohoku" });
    expect(within(route).getByText("L2TP/IPsec · vpn.example.jp · route open")).toBeVisible();
    const connections = within(route).getByRole("list", { name: "Connections using this profile" });
    expect(within(connections).getByText("lab")).toBeVisible();
  });

  it("only lists the connections, and says the profile is attached to a connection in Connections", async () => {
    render(<VPNPanel api={buildApi()} />);

    const route = await screen.findByRole("article", { name: "tohoku" });
    expect(within(route).queryByRole("combobox")).toBeNull();
    expect(within(route).queryByRole("button", { name: /lab/ })).toBeNull();
    expect(within(route).getByText(/change "VPN profile" on its sshc tab/)).toBeVisible();
  });

  it("says in its own words why this machine cannot open routes, and adds Docker's words as details", async () => {
    const detail = 'docker is not available: exec: "docker": executable file not found in $PATH';
    const api = buildApi({
      vpnOverview: vi.fn().mockResolvedValue(
        overview({ available: false, unavailable: "vpn_docker_missing", detail, profiles: [] }),
      ),
    });

    render(<VPNPanel api={api} />);

    expect(await screen.findByText("Docker was not found. VPN routes need Docker. Install Docker Desktop or similar.")).toBeVisible();
    expect(screen.getByText(`Details: ${detail}`)).toBeVisible();
  });

  it("tells a stopped Docker apart from a missing one", async () => {
    const api = buildApi({
      vpnOverview: vi.fn().mockResolvedValue(
        overview({ available: false, unavailable: "vpn_docker_not_running", detail: "Cannot connect to the Docker daemon", profiles: [] }),
      ),
    });

    render(<VPNPanel api={api} />);

    expect(await screen.findByText(/^Docker is not running\./)).toBeVisible();
  });

  it("sends the secrets with the profile and never shows them again", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await fillWireGuardProfile(user, form);
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(createVPNProfile).toHaveBeenCalledWith(
      {
        name: "lab",
        backend: "wireguard",
        wireguard: {
          server: "vpn.example.jp:51820",
          peerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=",
          address: "10.0.0.2/32",
        },
      },
      { wireguardPrivateKey: "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA=" },
    );
    await waitFor(() => expect(within(form).getByLabelText("Private key")).toHaveValue(""));
    expect(within(form).getByLabelText("Name")).toHaveValue("");
    expect(within(form).getByLabelText("VPN server")).toHaveValue("");
    expect(createVPNProfile.mock.calls[0]?.[0]).not.toHaveProperty("dns");
  });

  it("sends the VPN's own DNS servers, for connections whose HostName is a name", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await fillWireGuardProfile(user, form);
    await user.type(within(form).getByLabelText("DNS inside the VPN"), "10.9.9.53, 10.9.9.54");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(createVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({ dns: ["10.9.9.53", "10.9.9.54"] }),
      expect.anything(),
    );
    expect(createVPNProfile.mock.calls[0]?.[0]).not.toHaveProperty("target");
  });

  it("waits for approval without sending a second answer when the device asks nothing", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "approve");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    const [profile] = createVPNProfile.mock.calls[0] ?? [];
    expect(profile.openconnect).toEqual(
      expect.objectContaining({ secondFactor: "approve" }),
    );
    expect(profile.openconnect).not.toHaveProperty("approvalWord");
  });

  it("sends the word the device asks for when one is given", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "approve");
    await user.type(within(form).getByLabelText("Word to send as the second answer"), "push");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(createVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({
        openconnect: expect.objectContaining({ secondFactor: "approve", approvalWord: "push" }),
      }),
      { openconnectPassword: "a password" },
    );
  });

  it("carries the TOTP seed only when the second factor uses one", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "totp");
    // 種を入れるまでは保存できない。
    expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled();
    await user.type(within(form).getByLabelText("Second factor TOTP seed"), "GEZDGNBVGY3TQOJQ");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(createVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({ openconnect: expect.objectContaining({ secondFactor: "totp" }) }),
      { openconnectPassword: "a password", openconnectTotpSecret: "GEZDGNBVGY3TQOJQ" },
    );
  });

  it("says what it is waiting for while a route is being opened, and stops saying it once it is up", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const starting = overview({
        profiles: [{
          profile: tohokuProfile,
          running: true,
          relaySocket: "",
          connections: [],
          phase: "image",
        }],
      });
      const vpnOverview = vi.fn().mockResolvedValueOnce(starting).mockResolvedValue(overview());
      render(<VPNPanel api={buildApi({ vpnOverview })} />);

      expect(await screen.findByText(/building the image/)).toBeVisible();

      // 読み直しの間隔を越えるまで時計を進める。遅い機械でも、読み直しが
      // 始まる前に待ちを打ち切らない。
      await act(async () => {
        await vi.advanceTimersByTimeAsync(routeProgressIntervalMs * 2);
      });

      await waitFor(() => expect(screen.getByText(/route open/)).toBeVisible(), { timeout: 5000 });
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

  it("says what is wrong with a new name in the name field, without asking the engine", async () => {
    const user = userEvent.setup();
    const renameVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ renameVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Rename" }));
    const dialog = screen.getByRole("dialog");
    await user.clear(within(dialog).getByLabelText("New name"));
    await user.type(within(dialog).getByLabelText("New name"), "tohoku office");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));

    expect(within(dialog).getByLabelText("New name")).toHaveAccessibleDescription(
      "This is not written in a form this field accepts.",
    );
    expect(renameVPNProfile).not.toHaveBeenCalled();
  });

  it("names the field when the engine refuses a new name", async () => {
    const user = userEvent.setup();
    const renameVPNProfile = vi.fn().mockRejectedValue(
      new ApiError("vpn_profile_invalid", 400, {
        code: "vpn_profile_invalid", message: "request rejected", field: "name", reason: "format",
      }),
    );
    render(<VPNPanel api={buildApi({ renameVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Rename" }));
    const dialog = screen.getByRole("dialog");
    await user.clear(within(dialog).getByLabelText("New name"));
    await user.type(within(dialog).getByLabelText("New name"), "tains");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Name: This is not written in a form this field accepts.");
  });

  it("keeps a newer answer when an older status reading arrives late", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      let answerLateReading: (value: VPNOverview) => void = () => undefined;
      const lateReading = new Promise<VPNOverview>((resolve) => {
        answerLateReading = resolve;
      });
      const vpnOverview = vi.fn().mockResolvedValueOnce(startingOverview()).mockReturnValueOnce(lateReading);
      const stopVPNSession = vi.fn().mockResolvedValue(stoppedOverview());
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(<VPNPanel api={buildApi({ vpnOverview, stopVPNSession })} />);
      expect(await screen.findByText(/building the image/)).toBeVisible();

      // 読み直しが始まり、その応答が返る前に、利用者が経路を止める。
      await act(async () => {
        await vi.advanceTimersByTimeAsync(routeProgressIntervalMs * 2);
      });
      await waitFor(() => expect(vpnOverview).toHaveBeenCalledTimes(2), { timeout: 5000 });
      await user.click(screen.getByRole("button", { name: "Disconnect" }));
      await waitFor(() => expect(screen.getByText(/stopped/)).toBeVisible());

      await act(async () => {
        answerLateReading(startingOverview());
        await lateReading;
      });

      expect(screen.getByText(/stopped/)).toBeVisible();
      expect(screen.queryByText(/building the image/)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not read the status again while another operation is still running", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const vpnOverview = vi.fn().mockResolvedValue(startingOverview());
      const stopVPNSession = vi.fn().mockReturnValue(new Promise<VPNOverview>(() => undefined));
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(<VPNPanel api={buildApi({ vpnOverview, stopVPNSession })} />);
      expect(await screen.findByText(/building the image/)).toBeVisible();

      await user.click(screen.getByRole("button", { name: "Disconnect" }));
      const readingsBefore = vpnOverview.mock.calls.length;
      await act(async () => {
        vi.advanceTimersByTime(6000);
        await Promise.resolve();
      });

      expect(vpnOverview).toHaveBeenCalledTimes(readingsBefore);
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps reading the status while a route is being opened, to show how far it got", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const vpnOverview = vi.fn().mockResolvedValueOnce(stoppedOverview()).mockResolvedValue(startingOverview());
      const startVPNSession = vi.fn().mockReturnValue(new Promise<VPNOverview>(() => undefined));
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(<VPNPanel api={buildApi({ vpnOverview, startVPNSession })} />);
      const route = await screen.findByRole("article", { name: "tohoku" });

      await user.click(within(route).getByRole("button", { name: "Connect" }));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(routeProgressIntervalMs * 2);
      });

      expect(await screen.findByText(/building the image/, undefined, { timeout: 5000 })).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps what was typed, secrets included, when the engine refuses to save", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockRejectedValue(
      new ApiError("vpn_profile_exists", 409, { code: "vpn_profile_exists", message: "request rejected" }),
    );
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await fillWireGuardProfile(user, form);
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await screen.findByText(/already exists/)).toBeVisible();
    expect(within(form).getByLabelText("Name")).toHaveValue("lab");
    expect(within(form).getByLabelText("Private key")).toHaveValue(privateKey);
    expect(within(form).getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("forgets the secrets of the previous type when the type is switched", async () => {
    const user = userEvent.setup();
    render(<VPNPanel api={buildApi()} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.type(within(form).getByLabelText("Private key"), privateKey);
    await user.selectOptions(within(form).getByLabelText("Type"), "l2tp_ipsec");
    await user.selectOptions(within(form).getByLabelText("Type"), "wireguard");

    expect(within(form).getByLabelText("Private key")).toHaveValue("");
  });

  it("says which field is wrong before sending, next to that field", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await fillWireGuardProfile(user, form);
    await user.type(within(form).getByLabelText("DNS inside the VPN"), "dns.example.jp");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    const resolvers = within(form).getByLabelText("DNS inside the VPN");
    expect(resolvers).toHaveAttribute("aria-invalid", "true");
    expect(resolvers).toHaveAccessibleDescription("Not an IPv4 address.");
    expect(createVPNProfile).not.toHaveBeenCalled();
  });

  // 送る前の検査（validateAPIRequest）は、長さや形の違反を項目の名前なしで断る。その前に
  // フォームが項目ごとに理由を出す。
  it("names the private key when it is not a WireGuard key, instead of failing without a field", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.type(within(form).getByLabelText("Name"), "lab");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp:51820");
    await user.type(within(form).getByLabelText("Peer public key"), peerPublicKey);
    await user.type(within(form).getByLabelText("Private key"), privateKey.slice(1));
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(within(form).getByRole("alert")).toHaveTextContent("This is not written in a form this field accepts.");
    expect(createVPNProfile).not.toHaveBeenCalled();
  });

  it("names the word sent for approval when it is longer than the engine keeps", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.selectOptions(within(form).getByLabelText("Second factor"), "approve");
    await user.type(within(form).getByLabelText("Word to send as the second answer"), "p".repeat(33));
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(within(form).getByLabelText("Word to send as the second answer")).toHaveAccessibleDescription(
      "Too long: up to 32 characters.",
    );
    expect(createVPNProfile).not.toHaveBeenCalled();
  });

  it("names a password longer than the API accepts", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await user.selectOptions(within(form).getByLabelText("Type"), "l2tp_ipsec");
    await user.type(within(form).getByLabelText("Name"), "office");
    await user.type(within(form).getByLabelText("VPN server"), "vpn.example.jp");
    await user.type(within(form).getByLabelText("VPN username"), "tester");
    await user.click(within(form).getByLabelText("VPN password"));
    await user.paste("x".repeat(257));
    await user.type(within(form).getByLabelText("IPsec pre-shared key"), "a key");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(within(form).getByRole("alert")).toHaveTextContent("Too long: up to 256 characters.");
    expect(createVPNProfile).not.toHaveBeenCalled();
  });

  it("puts the limit into the reason when a value is too long", async () => {
    const user = userEvent.setup();
    render(<VPNPanel api={buildApi()} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await fillWireGuardProfile(user, form);
    await user.type(within(form).getByLabelText("Name"), "x".repeat(48));
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(within(form).getByLabelText("Name")).toHaveAccessibleDescription("Too long: up to 48 characters.");
  });

  it("shows the field the engine refused next to that field", async () => {
    const user = userEvent.setup();
    const createVPNProfile = vi.fn().mockRejectedValue(
      new ApiError("vpn_secrets_missing", 400, {
        code: "vpn_secrets_missing",
        message: "request rejected",
        field: "secrets.wireguardPrivateKey",
        reason: "format",
      }),
    );
    render(<VPNPanel api={buildApi({ createVPNProfile })} />);
    const form = await screen.findByRole("region", { name: "Add a VPN profile" });

    await fillWireGuardProfile(user, form);
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await within(form).findByText("This is not written in a form this field accepts.")).toBeVisible();
    expect(screen.getByText("Some values were not accepted. See the reason next to each field.")).toBeVisible();
    expect(within(form).getByLabelText("Private key")).toHaveValue(privateKey);
  });

  it("says why the route did not come up and offers its logs", async () => {
    const user = userEvent.setup();
    const startVPNSession = vi.fn().mockRejectedValue(
      new ApiError("vpn_session_failed", 409, {
        code: "vpn_session_failed",
        message: "request rejected",
        reason: "handshake_timeout",
      }),
    );
    const vpnLogs = vi.fn().mockResolvedValue({ lines: "no handshake" });
    render(<VPNPanel api={buildApi({ startVPNSession, vpnLogs })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Connect" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Connecting to the VPN failed. The handshake failed. Check the keys and the server.",
    );
    await user.click(screen.getByRole("button", { name: "Show the logs" }));
    expect(vpnLogs).toHaveBeenCalledWith("tohoku");
  });

  it("points to the logs when the engine did not say why the route failed", async () => {
    const user = userEvent.setup();
    const startVPNSession = vi.fn().mockRejectedValue(
      new ApiError("vpn_session_failed", 409, { code: "vpn_session_failed", message: "request rejected" }),
    );
    render(<VPNPanel api={buildApi({ startVPNSession })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Connect" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The reason could not be read. Check the logs.");
  });
  it("opens the saved values for editing, keeps the name, and keeps the stored secrets when left blank", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Edit" }));
    const form = screen.getByRole("region", { name: "Edit tohoku" });
    expect(within(form).getByLabelText("Name")).toHaveValue("tohoku");
    expect(within(form).getByLabelText("Name")).toBeDisabled();
    expect(within(form).getByLabelText("VPN server")).toHaveValue("vpn.example.jp");
    expect(within(form).getByLabelText("DNS inside the VPN")).toHaveValue("10.9.9.53");
    expect(within(form).getByLabelText("VPN password")).toHaveValue("");
    expect(within(form).getAllByText("Leave blank to keep the stored value.")).toHaveLength(2);

    await user.clear(within(form).getByLabelText("VPN server"));
    await user.type(within(form).getByLabelText("VPN server"), "vpn2.example.jp");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(
      { name: "tohoku", backend: "l2tp_ipsec", dns: ["10.9.9.53"], l2tp: { server: "vpn2.example.jp", username: "tester" } },
      {},
    );
    await waitFor(() => expect(screen.queryByRole("region", { name: "Edit tohoku" })).toBeNull());
    expect(screen.getByRole("article", { name: "tohoku" })).toBeVisible();
  });

  it("sends only the secrets that were typed while editing", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Edit" }));
    const form = screen.getByRole("region", { name: "Edit tohoku" });
    await user.type(within(form).getByLabelText("VPN password"), "a new password");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(expect.objectContaining({ name: "tohoku" }), {
      l2tpPassword: "a new password",
    });
  });

  it("asks for the new type's secrets when the type is changed while editing", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Edit" }));
    const form = screen.getByRole("region", { name: "Edit tohoku" });
    await user.selectOptions(within(form).getByLabelText("Type"), "openconnect");

    expect(within(form).queryByText("Leave blank to keep the stored value.")).toBeNull();
    expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled();
    await user.type(within(form).getByLabelText("VPN password"), "a password");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(saveVPNProfile).toHaveBeenCalledWith(
      expect.objectContaining({ backend: "openconnect", openconnect: expect.objectContaining({ server: "vpn.example.jp" }) }),
      { openconnectPassword: "a password" },
    );
  });

  it("puts the edited profile back as it was when editing is cancelled", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockResolvedValue(overview());
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Edit" }));
    await user.click(within(screen.getByRole("region", { name: "Edit tohoku" })).getByRole("button", { name: "Cancel" }));

    expect(screen.getByRole("article", { name: "tohoku" })).toBeVisible();
    expect(saveVPNProfile).not.toHaveBeenCalled();
  });

  it("keeps the edit form open and shows the refused field when the engine refuses the change", async () => {
    const user = userEvent.setup();
    const saveVPNProfile = vi.fn().mockRejectedValue(
      new ApiError("vpn_secrets_missing", 409, {
        code: "vpn_secrets_missing",
        message: "request rejected",
        field: "secrets.ipsecPsk",
        reason: "required",
      }),
    );
    render(<VPNPanel api={buildApi({ saveVPNProfile })} />);
    const route = await screen.findByRole("article", { name: "tohoku" });

    await user.click(within(route).getByRole("button", { name: "Edit" }));
    const form = screen.getByRole("region", { name: "Edit tohoku" });
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(await within(form).findByText("Enter a value.")).toBeVisible();
    expect(screen.getByRole("region", { name: "Edit tohoku" })).toBeVisible();
  });
});

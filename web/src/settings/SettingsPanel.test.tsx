import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { IntegrationsApi } from "../api/integrations";
import { SettingsPanel } from "./SettingsPanel";

function buildApi(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    loginItem: vi.fn().mockResolvedValue({ enabled: false, supported: true }),
    setLoginItem: vi.fn().mockResolvedValue({ enabled: true, supported: true }),
    changeMasterPassword: vi.fn().mockResolvedValue({
      vault: {
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: [],
        minPassphraseLength: 12,
      },
      snapshotResealed: true,
    }),
    ...overrides,
  } as unknown as IntegrationsApi;
}

async function fillMasterPassword(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Current master password"), "the old one is long");
  await user.type(screen.getByLabelText("New master password"), "the new one is long");
  await user.type(screen.getByLabelText("Confirm new master password"), "the new one is long");
}

describe("SettingsPanel", () => {
  // 端末の選択という設定は無くなった。接続はこのアプリケーションの中で開く
  // ので、開く先を選ぶという問い自体が存在しない。
  it("no longer offers a connection application to choose", async () => {
    render(<SettingsPanel api={buildApi()} />);

    await screen.findByRole("region", { name: "Master password" });
    expect(screen.queryByRole("region", { name: "Default connection application" })).toBeNull();
    expect(screen.queryByLabelText("Open connections with")).toBeNull();
  });

  it("offers start at login off by default and saves an explicit toggle", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SettingsPanel api={api} />);

    const section = await screen.findByRole("region", { name: "Start at login" });
    const toggle = within(section).getByRole("checkbox", { name: "Start sshc when I log in" });
    expect(toggle).not.toBeChecked();
    await user.click(toggle);

    await waitFor(() => expect(api.setLoginItem).toHaveBeenCalledWith(true));
    expect(toggle).toBeChecked();
  });

  it("omits an unsupported login item but reports load and save failures", async () => {
    const unsupported = render(<SettingsPanel api={buildApi({
      loginItem: vi.fn().mockResolvedValue({ enabled: false, supported: false }),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    expect(screen.queryByRole("region", { name: "Start at login" })).not.toBeInTheDocument();
    unsupported.unmount();

    const failedLoad = render(<SettingsPanel api={buildApi({
      loginItem: vi.fn().mockRejectedValue(new Error("unavailable")),
    })} />);
    expect(await screen.findByText("The start-at-login setting could not be read.")).toBeInTheDocument();
    failedLoad.unmount();

    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      setLoginItem: vi.fn().mockRejectedValue(new Error("refused")),
    })} />);
    const toggle = await screen.findByRole("checkbox", { name: "Start sshc when I log in" });
    await user.click(toggle);
    expect(await screen.findByText("That could not be changed.")).toBeInTheDocument();
    expect(toggle).not.toBeChecked();
  });

  it("validates, changes the master password, reports the live snapshot, and clears every field", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SettingsPanel api={api} />);
    await screen.findByRole("region", { name: "Master password" });

    await user.type(screen.getByLabelText("Current master password"), "the old one is long");
    await user.type(screen.getByLabelText("New master password"), "the new one is long");
    expect(screen.getByRole("button", { name: "Change the master password" })).toBeDisabled();
    await user.type(screen.getByLabelText("Confirm new master password"), "the new one is long");
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    await waitFor(() =>
      expect(api.changeMasterPassword).toHaveBeenCalledWith("the old one is long", "the new one is long"),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(/live snapshot/i);
    expect(screen.getByLabelText("Current master password")).toHaveValue("");
    expect(screen.getByLabelText("New master password")).toHaveValue("");
    expect(screen.getByLabelText("Confirm new master password")).toHaveValue("");
  });

  it("reports a local-only change when the bucket snapshot was not resealed", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockResolvedValue({
        vault: { exists: true, unlocked: true, aliases: [], dedicatedKeyPassphrases: [] },
        snapshotResealed: false,
        snapshotProblem: "sync_failed",
      }),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByRole("status")).toHaveTextContent(/still opens with the old password/i);
  });

  it("reports a wrong current password and clears every secret after failure", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockRejectedValue(new ApiError("wrong_passphrase", 403, null)),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByText("That is not the current master password. Nothing was changed.")).toBeInTheDocument();
    expect(screen.getByLabelText("Current master password")).toHaveValue("");
    expect(screen.getByLabelText("New master password")).toHaveValue("");
    expect(screen.getByLabelText("Confirm new master password")).toHaveValue("");
  });

  it("reports a generic master-password failure and clears every secret", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockRejectedValue(new Error("write failed")),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByText("The master password could not be changed.")).toBeInTheDocument();
    expect(screen.getByLabelText("Current master password")).toHaveValue("");
    expect(screen.getByLabelText("New master password")).toHaveValue("");
    expect(screen.getByLabelText("Confirm new master password")).toHaveValue("");
  });

  it("can reveal the new master password only on request", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);
    await screen.findByRole("region", { name: "Master password" });

    expect(screen.getByLabelText("New master password")).toHaveAttribute("type", "password");
    await user.click(screen.getByRole("button", { name: "Show New master password" }));
    expect(screen.getByLabelText("New master password")).toHaveAttribute("type", "text");
  });
});

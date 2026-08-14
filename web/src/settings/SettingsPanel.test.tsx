import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { IntegrationsApi } from "../api/integrations";
import { SettingsPanel } from "./SettingsPanel";

function buildApi(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    desktopSettings: vi.fn().mockResolvedValue({ keepRunning: false }),
    terminalSettings: vi.fn().mockResolvedValue({}),
    setTerminalSettings: vi.fn().mockResolvedValue(undefined),
    setDesktopSettings: vi.fn().mockResolvedValue(undefined),
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
  // **書かれていなければ止める側に倒す。** 動かし続けるのは明示的な選択である。
  it("offers keeping the engine running, off unless it was chosen", async () => {
    const user = userEvent.setup();
    const setDesktopSettings = vi.fn().mockResolvedValue(undefined);
    render(<SettingsPanel api={buildApi({ setDesktopSettings })} />);

    const toggle = await screen.findByLabelText("Keep running after the window closes");
    expect(toggle).not.toBeChecked();

    await user.click(toggle);
    expect(setDesktopSettings).toHaveBeenCalledWith(true);
    await waitFor(() => expect(toggle).toBeChecked());
  });

  // 読めないときも止める側に倒す。**読めない設定を「続けろ」と解釈しない。**
  it("falls back to stopping when the setting cannot be read", async () => {
    render(<SettingsPanel api={buildApi({
      desktopSettings: vi.fn().mockRejectedValue(new Error("offline")),
    })} />);

    const toggle = await screen.findByLabelText("Keep running after the window closes");
    expect(toggle).not.toBeChecked();
  });
  // 開始位置は書いた綴りのまま送る。**home の綴りに展開して送らない**——
  // 展開するのはサーバーであり、保存されるのは書いた形である。
  it("saves the starting directory as it was written", async () => {
    const user = userEvent.setup();
    const setTerminalSettings = vi.fn().mockResolvedValue(undefined);
    render(<SettingsPanel api={buildApi({ setTerminalSettings })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    await user.type(within(region).getByLabelText("Starting directory"), "~/work");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(setTerminalSettings).toHaveBeenCalledWith({ startDirectory: "~/work" });
    expect(await within(region).findByText(/Saved/)).toBeVisible();
  });

  // **空欄は「設定されていない」であり、既定と同じ値ではない。** 空欄を
  // 既定の数字で埋めて送ると、それが metadata に焼き付き、既定を変えた日に
  // その人だけが黙って取り残される。
  it("sends nothing for the fields left empty", async () => {
    const user = userEvent.setup();
    const setTerminalSettings = vi.fn().mockResolvedValue(undefined);
    render(<SettingsPanel api={buildApi({ setTerminalSettings })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    await user.type(within(region).getByLabelText("Consoles open at once"), "4");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(setTerminalSettings).toHaveBeenCalledWith({ maxSessions: 4 });
  });

  // 保存されている値は編集できる形で出す。**既定へ丸めて見せない**——
  // 丸めた値を人がそのまま保存すると、選んでいない設定が書き込まれる。
  it("shows the stored numbers and leaves the unset ones blank", async () => {
    render(<SettingsPanel api={buildApi({
      terminalSettings: vi.fn().mockResolvedValue({ maxSessions: 4 }),
    })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    expect(await within(region).findByLabelText("Consoles open at once")).toHaveValue(4);
    expect(within(region).getByLabelText("Scrollback per console (bytes)")).toHaveValue(null);
    expect(within(region).getByLabelText("Starting directory")).toHaveValue("");
  });

  // 断られた理由をそのまま出す。**「保存できません」で終わらせない**——
  // 直すのは人であり、直すには何が悪いのかが要る。
  it("says which way the directory was refused", async () => {
    const user = userEvent.setup();
    const setTerminalSettings = vi.fn().mockRejectedValue(
      new ApiError("start_directory_missing", 400, {
        code: "start_directory_missing",
        message: "no",
      }),
    );
    render(<SettingsPanel api={buildApi({ setTerminalSettings })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    await user.type(within(region).getByLabelText("Starting directory"), "~/nowhere");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(await within(region).findByText("That directory does not exist.")).toBeVisible();
  });

  // 繋ぎっぱなしをまとめて片付ける入口。**エンジンは止めない。**
  it("closes every open connection at once", async () => {
    const user = userEvent.setup();
    const closeAll = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsPanel
        api={buildApi()}
        consoles={{
          sessions: [
            { id: "a", title: "one", kind: "shell", forwards: [] },
            { id: "b", title: "two", kind: "shell", forwards: [] },
          ] as never,
          busy: false,
          closeAll,
        }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    expect(within(region).getByText("2 open")).toBeVisible();
    await user.click(within(region).getByRole("button", { name: "Close every connection" }));

    expect(closeAll).toHaveBeenCalledTimes(1);
  });

  // 閉じるものが無いときに押せると、押した人は何かが起きたと思う。
  it("cannot be pressed when nothing is open", async () => {
    render(
      <SettingsPanel
        api={buildApi()}
        consoles={{ sessions: [], busy: false, closeAll: vi.fn() }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    expect(within(region).getByRole("button", { name: "Close every connection" })).toBeDisabled();
  });
});

import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { IntegrationsApi } from "../api/integrations";
import { SettingsPanel } from "./SettingsPanel";

function buildApi(overrides: Partial<IntegrationsApi> = {}): IntegrationsApi {
  return {
    terminalSettings: vi.fn().mockResolvedValue({}),
    engineSettings: vi.fn().mockResolvedValue({}),
    setEngineSettings: vi.fn(),
    setTerminalSettings: vi.fn().mockResolvedValue(undefined),
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
  it("does not accept terminal edits before initial settings finish loading", async () => {
    let finishLoading: (settings: { maxSessions: number }) => void = () => undefined;
    const terminalSettings = vi.fn(() => new Promise<{ maxSessions: number }>((resolve) => {
      finishLoading = resolve;
    }));
    render(<SettingsPanel api={buildApi({ terminalSettings })} />);

    const region = screen.getByRole("region", { name: "Terminal" });
    const sessions = within(region).getByLabelText("Consoles open at once");
    expect(sessions).toBeDisabled();
    expect(within(region).getByRole("status")).toHaveTextContent("Loading terminal settings");

    finishLoading({ maxSessions: 2 });
    await waitFor(() => expect(sessions).toBeEnabled());
    expect(sessions).toHaveValue(2);
  });

  it("waits for the application to refresh terminal state before reporting a save", async () => {
    const user = userEvent.setup();
    let finishRefresh: () => void = () => undefined;
    const refreshed = new Promise<void>((resolve) => {
      finishRefresh = resolve;
    });
    const onTerminalSettingsChange = vi.fn(() => refreshed);
    render(<SettingsPanel api={buildApi()} onTerminalSettingsChange={onTerminalSettingsChange} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    const sessions = within(region).getByLabelText("Consoles open at once");
    await waitFor(() => expect(sessions).toBeEnabled());
    await user.type(sessions, "2");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onTerminalSettingsChange).toHaveBeenCalledWith({ maxSessions: 2 }));
    expect(within(region).queryByText(/Saved/)).not.toBeInTheDocument();
    finishRefresh();
    expect(await within(region).findByText(/Saved/)).toBeVisible();
  });

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
    expect(await screen.findByRole("status")).toHaveTextContent(/live bucket snapshot/i);
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

    expect(await screen.findByRole("status")).toHaveTextContent(/still requires the old password/i);
  });

  it("reports a wrong current password and clears every secret after failure", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockRejectedValue(new ApiError("wrong_passphrase", 403, null)),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByText("The current master password is incorrect. Nothing was changed.")).toBeInTheDocument();
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

  it("sends nothing for the fields left empty", async () => {
    const user = userEvent.setup();
    const setTerminalSettings = vi.fn().mockResolvedValue(undefined);
    render(<SettingsPanel api={buildApi({ setTerminalSettings })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    await user.type(within(region).getByLabelText("Consoles open at once"), "4");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(setTerminalSettings).toHaveBeenCalledWith({ maxSessions: 4 });
  });

  it("shows both clipboard conveniences on by default and saves each disabled choice", async () => {
    const user = userEvent.setup();
    const setTerminalSettings = vi.fn().mockResolvedValue(undefined);
    render(<SettingsPanel api={buildApi({ setTerminalSettings })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    const copy = within(region).getByRole("checkbox", { name: "Copy selected text automatically" });
    const paste = within(region).getByRole("checkbox", { name: "Paste with right click" });
    expect(copy).toBeChecked();
    expect(paste).toBeChecked();

    await user.click(copy);
    await user.click(paste);
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(setTerminalSettings).toHaveBeenCalledWith({ copyOnSelect: false, rightClickPaste: false });
  });

  it("loads explicitly disabled clipboard choices", async () => {
    render(<SettingsPanel api={buildApi({
      terminalSettings: vi.fn().mockResolvedValue({ copyOnSelect: false, rightClickPaste: false }),
      engineSettings: vi.fn().mockResolvedValue({}),
      setEngineSettings: vi.fn(),
    })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    expect(await within(region).findByRole("checkbox", { name: "Copy selected text automatically" })).not.toBeChecked();
    expect(within(region).getByRole("checkbox", { name: "Paste with right click" })).not.toBeChecked();
  });

  it("saves not reconnecting as a choice, not as an empty field", async () => {
    const user = userEvent.setup();
    const setTerminalSettings = vi.fn().mockResolvedValue(undefined);
    render(<SettingsPanel api={buildApi({ setTerminalSettings })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    await user.selectOptions(
      within(region).getByLabelText("Reconnect after a dropped connection"),
      "0",
    );
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(setTerminalSettings).toHaveBeenCalledWith({ reconnect: 0 });
  });

  it("loads a stored choice of never reconnecting", async () => {
    render(<SettingsPanel api={buildApi({
      terminalSettings: vi.fn().mockResolvedValue({ reconnect: 0 }),
      engineSettings: vi.fn().mockResolvedValue({}),
      setEngineSettings: vi.fn(),
    })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    expect(await within(region).findByLabelText("Reconnect after a dropped connection"))
      .toHaveValue("0");
  });

  it("shows the stored numbers and leaves the unset ones blank", async () => {
    render(<SettingsPanel api={buildApi({
      terminalSettings: vi.fn().mockResolvedValue({ maxSessions: 4 }),
      engineSettings: vi.fn().mockResolvedValue({}),
      setEngineSettings: vi.fn(),
    })} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    expect(await within(region).findByLabelText("Consoles open at once")).toHaveValue(4);
    expect(within(region).getByLabelText("Scrollback per console (bytes)")).toHaveValue(null);
    expect(within(region).getByLabelText("Starting directory")).toHaveValue("");
  });

  it("updates the terminal preview from the appearance controls", async () => {
    const user = userEvent.setup();
    const { container } = render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    await user.selectOptions(within(region).getByLabelText("Colour scheme"), "nord");
    await user.type(within(region).getByLabelText("Font size"), "16");

    const preview = container.querySelector("[data-terminal-preview]");
    expect(preview).toHaveAttribute("data-term-palette", "nord");
    expect(preview).toHaveStyle({ fontSize: "16px" });
  });

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

    expect(await within(region).findByText("The specified directory does not exist.")).toBeVisible();
  });

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
    await user.click(screen.getByRole("button", { name: "Close them all" }));

    expect(closeAll).toHaveBeenCalledTimes(1);
  });

  it("does not end every connection on the first press", async () => {
    const user = userEvent.setup();
    const closeAll = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsPanel
        api={buildApi()}
        consoles={{
          sessions: [{ id: "a", title: "one", kind: "shell", forwards: [] }] as never,
          busy: false,
          closeAll,
        }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    await user.click(within(region).getByRole("button", { name: "Close every connection" }));

    expect(closeAll).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("clears finished rows without asking", async () => {
    const user = userEvent.setup();
    const closeAll = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsPanel
        api={buildApi()}
        consoles={{
          sessions: [
            { id: "a", title: "one", kind: "shell", forwards: [], exited: { code: 0, signal: "", at: "x" } },
          ] as never,
          busy: false,
          closeAll,
        }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    await user.click(within(region).getByRole("button", { name: "Close every connection" }));

    expect(closeAll).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

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

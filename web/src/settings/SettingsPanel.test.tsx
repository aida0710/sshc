import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { LanguageProvider } from "../i18n/context";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient, ApiError } from "../api/client";
import { fakeSettingsServer, settingsDocument, testMachineId } from "../testing/settingsServer";
import { vaultStatus } from "../testing/vaultStatus";
import { refreshSettings, resetSettingsForTests, type SettingsPreset } from "./settingsStore";
import { SettingsPanel } from "./SettingsPanel";
import type { SettingsPanelApi } from "./SettingsPanel";

let server: ReturnType<typeof fakeSettingsServer>;

beforeEach(() => {
  server = fakeSettingsServer();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function buildApi(overrides: Partial<SettingsPanelApi> = {}): SettingsPanelApi {
  return {
    passwordVault: vi.fn().mockResolvedValue(vaultStatus()),
    localShellProfiles: vi.fn().mockResolvedValue({
      profiles: [
        { id: "default", label: "bash (default)", path: "/bin/bash", arguments: [], default: true },
        { id: "fish", label: "fish", path: "/usr/bin/fish", arguments: [], default: false },
      ],
    }),
    changeMasterPassword: vi.fn().mockResolvedValue({ vault: vaultStatus() }),
    ...overrides,
  } as unknown as SettingsPanelApi;
}

async function fillMasterPassword(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText("Current master password"), "the old one is long");
  await user.type(screen.getByLabelText("New master password"), "the new one is long");
  await user.type(screen.getByLabelText("Confirm new master password"), "the new one is long");
}

// openTerminalTab moves Settings > Terminal to one of its tabs; on a computer each group of fields is a tab.
function openTerminalTab(region: HTMLElement, name: "Display" | "Connection" | "Behaviour") {
  fireEvent.click(within(region).getByRole("tab", { name }));
}

// presetSaved is the preset id as the engine keeps it after the writes so far.
function presetSaved(id = "default"): SettingsPreset | undefined {
  return server.document().presets.find((preset) => preset.id === id);
}

// withSettings starts the engine's settings from patch, as another machine left them.
function withSettings(patch: Parameters<typeof fakeSettingsServer>[0]) {
  vi.restoreAllMocks();
  server = fakeSettingsServer(patch);
}

// holdWrites keeps every settings write waiting until the test lets it through.
function holdWrites(): () => void {
  let release: () => void = () => undefined;
  const mutate = vi.mocked(apiClient.mutate).getMockImplementation();
  vi.mocked(apiClient.mutate).mockImplementation((path, init) => new Promise((resolve, reject) => {
    release = () => { void mutate?.(path, init).then(resolve, reject); };
  }));
  return () => release();
}

// refuseWrites answers every settings write with the engine's problem code.
function refuseWrites(code: string) {
  vi.mocked(apiClient.mutate).mockRejectedValue(new ApiError(code, 400, { code, message: "no" }));
}

describe("SettingsPanel", () => {
  it("renders only the selected settings page", () => {
    render(<SettingsPanel api={buildApi()} page="Terminal" />);

    expect(screen.getByRole("heading", { name: "Terminal" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Terminal" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Machines" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Notifications" })).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Settings" })).not.toBeInTheDocument();
  });

  it("lists the pages in the global settings, the preset being edited and the machines", () => {
    render(<SettingsPanel api={buildApi()} page="Terminal" onNavigatePage={vi.fn()} />);

    const navigation = screen.getByRole("navigation", { name: "Settings" });
    expect(within(navigation).getByText("Global")).toBeVisible();
    expect(within(navigation).getByRole("combobox", { name: "Preset to edit" })).toHaveValue("default");
    expect(within(navigation).getByText("In use here")).toBeVisible();
    expect(within(navigation).getAllByRole("link").map((link) => link.textContent)).toEqual([
      "General", "Appearance", "Terminal", "SFTP", "Keyboard shortcuts", "Notifications", "Vault", "Machines", "Open connections",
    ]);
  });

  it("edits the preset chosen above the pages and offers to use it on this machine", async () => {
    const user = userEvent.setup();
    withSettings({ presets: [{ id: "default" }, { id: "phone", name: "Phone", terminal: { maxSessions: 3 } }] });
    render(<SettingsPanel api={buildApi()} page="Terminal" onNavigatePage={vi.fn()} />);
    const navigation = screen.getByRole("navigation", { name: "Settings" });

    await user.selectOptions(within(navigation).getByRole("combobox", { name: "Preset to edit" }), "phone");
    const region = screen.getByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    expect(within(region).getByLabelText("Sessions open at once")).toHaveValue(3);
    await user.clear(within(region).getByLabelText("Sessions open at once"));
    await user.type(within(region).getByLabelText("Sessions open at once"), "4");
    await user.click(within(region).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(presetSaved("phone")?.terminal).toEqual({ maxSessions: 4 }));
    expect(presetSaved("default")).toEqual({ id: "default" });

    await user.click(within(navigation).getByRole("button", { name: "Use on this machine" }));
    await waitFor(() => expect(server.document().machines[0]?.settings.presetId).toBe("phone"));
    expect(await within(navigation).findByText("In use here")).toBeVisible();
  });

  it("makes a new preset from the one open and names it", async () => {
    const user = userEvent.setup();
    withSettings({ presets: [{ id: "default", terminal: { fontSize: 15 } }] });
    render(<SettingsPanel api={buildApi()} page="Terminal" onNavigatePage={vi.fn()} />);
    const navigation = screen.getByRole("navigation", { name: "Settings" });

    await user.click(within(navigation).getByRole("button", { name: "Actions for Standard" }));
    await user.click(screen.getByRole("menuitem", { name: "New preset" }));
    await user.type(screen.getByLabelText("Preset name"), "Phone");
    await user.click(screen.getByRole("button", { name: "New preset" }));

    await waitFor(() => expect(server.document().presets).toHaveLength(2));
    expect(server.document().presets[1]).toMatchObject({ name: "Phone", terminal: { fontSize: 15 } });
    expect(within(navigation).getByRole("combobox", { name: "Preset to edit" })).toHaveDisplayValue("Phone");
  });

  it("puts the preset picker above a preset page on a phone, where the list of pages is not shown", () => {
    vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
      matches: true, media: query, onchange: null,
      addEventListener: vi.fn(), removeEventListener: vi.fn(), addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(),
    })));
    render(<SettingsPanel api={buildApi()} page="SFTP" onNavigatePage={vi.fn()} />);

    expect(screen.getByRole("combobox", { name: "Preset to edit" })).toBeVisible();
  });

  it("loads the twelve-hour default and saves a timed Vault lock in the preset being edited", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Vault" />);

    const region = screen.getByRole("region", { name: "Vault auto-lock" });
    const mode = within(region).getByLabelText("Vault auto-lock");
    await waitFor(() => expect(mode).toBeEnabled());
    expect(mode).toHaveValue("idle");
    expect(within(region).getByLabelText("Time")).toHaveValue(12);
    expect(within(region).getByLabelText("Unit")).toHaveValue("hours");

    await user.clear(within(region).getByLabelText("Time"));
    await user.type(within(region).getByLabelText("Time"), "45");
    await user.selectOptions(within(region).getByLabelText("Unit"), "minutes");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(presetSaved()?.vaultAutoLock).toEqual({ mode: "idle", value: 45, unit: "minutes" }));
    expect(await within(region).findByText(/Auto-lock applies now/)).toBeVisible();
  });

  it("shows a persistent warning and saves restart-only locking", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Vault" />);

    const region = screen.getByRole("region", { name: "Vault auto-lock" });
    const mode = within(region).getByLabelText("Vault auto-lock");
    await waitFor(() => expect(mode).toBeEnabled());
    await user.selectOptions(mode, "restart");

    expect(within(region).queryByLabelText("Time")).toBeNull();
    expect(within(region).getByText(/remains unlocked until sshc is restarted/i)).toBeVisible();
    await user.click(within(region).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(presetSaved()?.vaultAutoLock).toEqual({ mode: "restart" }));
  });

  it("restores restart-only locking and rejects a duration outside 1 to 999", async () => {
    const user = userEvent.setup();
    withSettings({ presets: [{ id: "default", vaultAutoLock: { mode: "restart" } }] });
    render(<SettingsPanel api={buildApi()} page="Vault" />);

    const region = screen.getByRole("region", { name: "Vault auto-lock" });
    const mode = within(region).getByLabelText("Vault auto-lock");
    await waitFor(() => expect(mode).toHaveValue("restart"));
    await user.selectOptions(mode, "idle");
    await user.clear(within(region).getByLabelText("Time"));
    await user.type(within(region).getByLabelText("Time"), "1000");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(server.writesTo("/api/v1/settings/presets/default")).toEqual([]);
    expect(within(region).getByText(/whole number from 1 to 999/i)).toBeVisible();
  });

  it("locks the auto-lock fields of a passwordless Vault, says why, and keeps the saved choice", () => {
    withSettings({ presets: [{ id: "default", vaultAutoLock: { mode: "idle", value: 30, unit: "minutes" } }] });
    render(<SettingsPanel passwordless api={buildApi()} page="Vault" />);

    const region = screen.getByRole("region", { name: "Vault auto-lock" });
    expect(within(region).getByLabelText("Vault auto-lock")).toBeDisabled();
    expect(within(region).getByLabelText("Time")).toHaveValue(30);
    expect(within(region).getByLabelText("Time")).toBeDisabled();
    expect(within(region).getByLabelText("Unit")).toBeDisabled();
    expect(within(region).getByText(/without a master password never locks automatically/i)).toBeVisible();
    expect(within(region).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("tells a passwordless Vault that auto-lock does not apply instead of warning about restart-only locking", () => {
    withSettings({ presets: [{ id: "default", vaultAutoLock: { mode: "restart" } }] });
    render(<SettingsPanel passwordless api={buildApi()} page="Vault" />);

    const region = screen.getByRole("region", { name: "Vault auto-lock" });
    expect(within(region).getByLabelText("Vault auto-lock")).toHaveValue("restart");
    expect(within(region).getByText(/without a master password never locks automatically/i)).toBeVisible();
    expect(within(region).queryByText(/remains unlocked until sshc is restarted/i)).toBeNull();
  });

  it("shows the Vault save working until the engine answers and accepts no second press", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Vault" />);
    const release = holdWrites();

    const region = screen.getByRole("region", { name: "Vault auto-lock" });
    const save = within(region).getByRole("button", { name: "Save" });
    await user.click(save);

    expect(save).toHaveAttribute("aria-busy", "true");
    await user.click(save);
    release();
    expect(await within(region).findByText(/Auto-lock applies now/)).toBeVisible();
    expect(save).not.toHaveAttribute("aria-busy");
    expect(server.writesTo("/api/v1/settings/presets/default")).toHaveLength(1);
  });

  it("keeps the notification sound and volume in the preset being edited", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Notifications" />);
    const region = screen.getByRole("region", { name: "Notifications" });

    await user.selectOptions(within(region).getByLabelText("Notification sound"), "pulse");
    fireEvent.change(within(region).getByLabelText("Notification volume"), { target: { value: "35" } });

    await waitFor(() => expect(presetSaved()?.interface?.notificationSound).toEqual({ sound: "pulse", volume: 35 }), { timeout: 3000 });
  });

  it("does not accept terminal edits before the settings arrive", async () => {
    resetSettingsForTests();
    render(<SettingsPanel api={buildApi()} page="Terminal" />);

    const region = screen.getByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    const sessions = within(region).getByLabelText("Sessions open at once");
    expect(sessions).toBeDisabled();
    expect(within(region).getByRole("status")).toHaveTextContent("Loading terminal settings");

    resetSettingsForTests(settingsDocument({ presets: [{ id: "default", terminal: { maxSessions: 2 } }] }));
    await waitFor(() => expect(sessions).toBeEnabled());
    expect(sessions).toHaveValue(2);
  });

  it("says the terminal settings could not be read and keeps the save closed", async () => {
    resetSettingsForTests();
    vi.mocked(apiClient.read).mockRejectedValue(new Error("engine restarting"));
    await refreshSettings();
    render(<SettingsPanel api={buildApi()} page="Terminal" />);

    const region = screen.getByRole("region", { name: "Terminal" });
    expect(within(region).getByText("Terminal settings could not be loaded. They cannot be saved until they load.")).toBeVisible();
    expect(within(region).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("saves the starting directory as it was written", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Behaviour");
    await user.type(within(region).getByLabelText(/^Starting directory/), "~/work");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(presetSaved()?.terminal).toEqual({ startDirectory: "~/work" }));
    expect(await within(region).findByText(/^Saved\./)).toBeVisible();
  });

  it("sends nothing for the fields left empty", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    await user.type(within(region).getByLabelText("Sessions open at once"), "4");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(presetSaved()?.terminal).toEqual({ maxSessions: 4 }));
  });

  it("saves both scrollback scopes, shell profile and the JIS yen key, typed on two tabs", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    await waitFor(() => expect(within(region).getByLabelText("Engine replay buffer (bytes)")).toBeEnabled());
    await user.type(within(region).getByLabelText("Engine replay buffer (bytes)"), "65536");
    await user.type(within(region).getByLabelText("Browser scrollback (lines)"), "12000");
    await user.type(within(region).getByLabelText("Terminals kept hidden"), "20");
    openTerminalTab(region, "Behaviour");
    await waitFor(() => expect(within(region).getByLabelText(/^Default local shell/)).toBeEnabled());
    await user.selectOptions(within(region).getByLabelText(/^Default local shell/), "fish");
    await user.click(within(region).getByRole("checkbox", { name: /Send the JIS/ }));
    await user.click(within(region).getByRole("checkbox", { name: /Use font ligatures/ }));
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(presetSaved()?.terminal).toEqual({
      scrollbackBytes: 65536,
      browserScrollbackLines: 12000,
      retainedTerminals: 20,
      localShellProfile: "fish",
      jisYenBackslash: true,
      ligatures: true,
    }));
  });

  it("edits the chosen preset, with the engine's and the browser's limits apart and the global settings elsewhere", async () => {
    render(<SettingsPanel api={buildApi()} page="Terminal" />);

    expect(screen.getByText("Edits the chosen preset.")).toBeVisible();
    const terminal = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(terminal, "Connection");
    expect(within(terminal).getByLabelText("Engine replay buffer (bytes)")).toHaveAttribute("max", "4194304");
    expect(within(terminal).getByLabelText("Browser scrollback (lines)")).toHaveAttribute("max", "100000");
    expect(within(terminal).getByLabelText("Sessions open at once")).toHaveAttribute("max", "200");
    expect(within(terminal).getByLabelText("Terminals kept hidden")).toHaveAttribute("max", "99");
    expect(within(terminal).queryByLabelText(/^Connection log/)).toBeNull();
    expect(within(terminal).queryByLabelText(/^Reconnect after a dropped connection/)).toBeNull();
  });

  it("shows both clipboard conveniences on by default and saves each disabled choice", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Behaviour");
    const copy = within(region).getByRole("checkbox", { name: "Copy selected text automatically" });
    const paste = within(region).getByRole("checkbox", { name: "Paste with right click" });
    expect(copy).toBeChecked();
    expect(paste).toBeChecked();

    await user.click(copy);
    await user.click(paste);
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(presetSaved()?.terminal).toEqual({ copyOnSelect: false, rightClickPaste: false }));
  });

  it("uses WebGL by default and persists an explicit disabled choice", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Behaviour");
    const webgl = await within(region).findByRole("checkbox", { name: "Use WebGL rendering" });
    expect(webgl).toBeChecked();
    await user.click(webgl);
    await user.click(within(region).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(presetSaved()?.terminal).toEqual({ webgl: false }));
  });

  it("loads explicitly disabled clipboard choices", async () => {
    withSettings({ presets: [{ id: "default", terminal: { copyOnSelect: false, rightClickPaste: false } }] });
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Behaviour");
    await waitFor(() => expect(within(region).getByRole("checkbox", { name: "Copy selected text automatically" })).not.toBeChecked());
    expect(within(region).getByRole("checkbox", { name: "Paste with right click" })).not.toBeChecked();
  });

  it("shows the stored numbers and leaves the unset ones blank", async () => {
    withSettings({ presets: [{ id: "default", terminal: { maxSessions: 4 } }] });
    render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    await waitFor(() => expect(within(region).getByLabelText("Sessions open at once")).toHaveValue(4));
    expect(within(region).getByLabelText("Engine replay buffer (bytes)")).toHaveValue(null);
    openTerminalTab(region, "Behaviour");
    expect(within(region).getByLabelText(/^Starting directory/)).toHaveValue("");
  });

  it("says which way the directory was refused", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);
    refuseWrites("start_directory_missing");

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Behaviour");
    await user.type(within(region).getByLabelText(/^Starting directory/), "~/nowhere");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(await within(region).findByText("The specified directory does not exist.")).toBeVisible();
  });

  it("keeps what was typed on one tab while another is open, and saves both", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Terminal" />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    const sessions = within(region).getByLabelText("Sessions open at once");
    await waitFor(() => expect(sessions).toBeEnabled());
    await user.type(sessions, "7");
    openTerminalTab(region, "Display");
    await user.type(within(region).getByLabelText("Font size"), "14");
    openTerminalTab(region, "Connection");

    expect(within(region).getByRole("tab", { name: "Connection" })).toHaveAttribute("aria-selected", "true");
    expect(within(region).getByLabelText("Sessions open at once")).toHaveValue(7);
    await user.click(within(region).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(presetSaved()?.terminal).toEqual({ maxSessions: 7, fontSize: 14 }));
  });

  it("opens the tab of the field a refused save is about", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Terminal" />);
    refuseWrites("start_directory_missing");

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Behaviour");
    const directory = within(region).getByLabelText(/^Starting directory/);
    await waitFor(() => expect(directory).toBeEnabled());
    await user.type(directory, "~/nowhere");
    openTerminalTab(region, "Display");
    await user.click(within(region).getByRole("button", { name: "Save" }));

    expect(await within(region).findByText("The specified directory does not exist.")).toBeVisible();
    expect(within(region).getByRole("tab", { name: "Behaviour" })).toHaveAttribute("aria-selected", "true");
    expect(within(region).getByLabelText(/^Starting directory/)).toHaveValue("~/nowhere");
  });

  it("shows the terminal save working while the fields wait, without dimming it with them", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);
    const release = holdWrites();

    const region = await screen.findByRole("region", { name: "Terminal" });
    openTerminalTab(region, "Connection");
    const sessions = within(region).getByLabelText("Sessions open at once");
    await user.type(sessions, "3");
    const save = within(region).getByRole("button", { name: "Save" });
    await user.click(save);

    expect(save).toHaveAttribute("aria-busy", "true");
    expect(sessions).toBeDisabled();
    expect(save.closest("fieldset:disabled")).toBeNull();
    release();
    expect(await within(region).findByText(/^Saved\./)).toBeVisible();
  });

  it("saves not reconnecting as a global choice, not as an empty field", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="General" />);

    const region = screen.getByRole("region", { name: "General" });
    await user.selectOptions(within(region).getByLabelText(/^Reconnect after a dropped connection/), "0");

    await waitFor(() => expect(server.document().global).toEqual({ terminal: { reconnect: 0 } }));
  });

  it("loads a stored choice of never reconnecting", () => {
    withSettings({ global: { terminal: { reconnect: 0 } } });
    render(<SettingsPanel api={buildApi()} page="General" />);

    expect(within(screen.getByRole("region", { name: "General" })).getByLabelText(/^Reconnect after a dropped connection/)).toHaveValue("0");
  });

  it("turns OSC 52 on for every machine and leaves the language to the browser when asked", async () => {
    const user = userEvent.setup();
    withSettings({ global: { language: "en" } });
    // The language is saved by LanguageProvider, as in the app.
    render(<LanguageProvider><SettingsPanel api={buildApi()} page="General" /></LanguageProvider>);
    const region = screen.getByRole("region", { name: "General" });

    await user.click(within(region).getByRole("checkbox", { name: "Allow writing to the clipboard" }));
    await waitFor(() => expect(server.document().global.terminal).toEqual({ osc52: true }));
    await user.selectOptions(within(region).getByLabelText("Display language"), "");
    await waitFor(() => expect(server.document().global.language).toBeUndefined());
  });

  it("lists this machine first and changes the preset and port of any machine", async () => {
    const user = userEvent.setup();
    const laptop = "1111111111111111111111111111111a";
    withSettings({
      presets: [{ id: "default" }, { id: "phone", name: "Phone" }],
      machines: [
        { id: testMachineId, settings: { name: "desk" }, facts: { os: "linux", version: "v0.59.0" } },
        { id: laptop, settings: { name: "laptop", port: 54447 }, facts: { os: "darwin" } },
      ],
    });
    render(<SettingsPanel api={buildApi()} page="Machines" />);
    const region = screen.getByRole("region", { name: "Machines" });

    expect(within(region).getByText("This machine")).toBeVisible();
    expect(within(region).getByText("Linux · sshc 0.59.0")).toBeVisible();
    const [, laptopPreset] = within(region).getAllByRole("combobox", { name: "Preset" });
    await user.selectOptions(laptopPreset!, "phone");
    await waitFor(() => expect(server.document().machines[1]?.settings).toEqual({ name: "laptop", port: 54447, presetId: "phone" }));

    const [, laptopPort] = within(region).getAllByRole("spinbutton", { name: "Port" });
    await user.clear(laptopPort!);
    await user.type(laptopPort!, "80{Enter}");
    expect(laptopPort).toHaveValue(54447);
    await user.clear(laptopPort!);
    await user.tab();
    await waitFor(() => expect(server.document().machines[1]?.settings).toEqual({ name: "laptop", presetId: "phone" }));
  });

  it("does not offer to take this machine off the list", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} page="Machines" />);

    await user.click(screen.getByRole("button", { name: "Actions for desk" }));

    expect(screen.getByRole("menuitem", { name: "Remove from the list" })).toBeDisabled();
  });

  it("requests browser notification permission from an explicit click and confirms delivery", async () => {
    const delivered: Array<{ title: string; options: NotificationOptions | undefined }> = [];
    class FakeNotification {
      static permission: NotificationPermission = "default";
      static requestPermission = vi.fn(async () => {
        FakeNotification.permission = "granted";
        return "granted" as const;
      });

      constructor(title: string, options?: NotificationOptions) {
        delivered.push({ title, options });
      }
    }
    vi.stubGlobal("Notification", FakeNotification);
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);

    const region = screen.getByRole("region", { name: "Notifications" });
    await user.click(within(region).getByRole("button", { name: "Enable notifications" }));

    await waitFor(() => expect(FakeNotification.requestPermission).toHaveBeenCalledTimes(1));
    expect(await within(region).findByRole("status")).toHaveTextContent("Notifications enabled");
    expect(within(region).getByRole("button", { name: "Send test notification" })).toBeVisible();
    expect(delivered).toEqual([{
      title: "sshc",
      options: {
        body: "Terminal and transfer notifications are ready.",
        tag: "sshc-notification-permission",
      },
    }]);
  });

  it("explains when browser notifications were blocked", () => {
    class FakeNotification {
      static permission: NotificationPermission = "denied";
      static requestPermission = vi.fn();
    }
    vi.stubGlobal("Notification", FakeNotification);
    render(<SettingsPanel api={buildApi()} />);

    const region = screen.getByRole("region", { name: "Notifications" });
    expect(within(region).getByText(/blocked for sshc/i)).toBeVisible();
    expect(within(region).queryByRole("button", { name: "Enable notifications" })).toBeNull();
  });

  it("no longer offers a connection application to choose", async () => {
    render(<SettingsPanel api={buildApi()} />);

    await screen.findByRole("region", { name: "Master password" });
    expect(screen.queryByRole("region", { name: "Default connection application" })).toBeNull();
    expect(screen.queryByLabelText("Open connections with")).toBeNull();
  });

  it("validates, changes the local master password, and clears every field", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<SettingsPanel api={api} />);
    await screen.findByRole("region", { name: "Master password" });

    await user.type(await screen.findByLabelText("Current master password"), "the old one is long");
    await user.type(screen.getByLabelText("New master password"), "the new one is long");
    expect(screen.getByRole("button", { name: "Change the master password" })).toBeDisabled();
    await user.type(screen.getByLabelText("Confirm new master password"), "the new one is long");
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    await waitFor(() =>
      expect(api.changeMasterPassword).toHaveBeenCalledWith("the old one is long", "the new one is long"),
    );
    expect((await screen.findByText(/remote snapshots were not rewritten/i)).closest("[role=\"status\"]")).not.toBeNull();
    expect(screen.getByLabelText("Current master password")).toHaveValue("");
    expect(screen.getByLabelText("New master password")).toHaveValue("");
    expect(screen.getByLabelText("Confirm new master password")).toHaveValue("");
  });

  it("reports a wrong current password and keeps every field so it can be fixed without retyping", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockRejectedValue(new ApiError("wrong_passphrase", 403, null)),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByText("The current master password is incorrect. Nothing was changed.")).toBeInTheDocument();
    expect(screen.getByLabelText("Current master password")).toHaveValue("the old one is long");
    expect(screen.getByLabelText("New master password")).toHaveValue("the new one is long");
    expect(screen.getByLabelText("Confirm new master password")).toHaveValue("the new one is long");
  });

  it("says how to reduce the local backups when there are too many to re-encrypt", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockRejectedValue(new ApiError("vault_backups_too_many", 409, null)),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByText(/too many local backups.*Delete old folders from ~\/\.ssh\/sshc\/backups/)).toBeInTheDocument();
  });

  it("reports a generic master-password failure and keeps every field", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi({
      changeMasterPassword: vi.fn().mockRejectedValue(new Error("write failed")),
    })} />);
    await screen.findByRole("region", { name: "Master password" });
    await fillMasterPassword(user);
    await user.click(screen.getByRole("button", { name: "Change the master password" }));

    expect(await screen.findByText("The master password could not be changed.")).toBeInTheDocument();
    expect(screen.getByLabelText("Current master password")).toHaveValue("the old one is long");
    expect(screen.getByLabelText("New master password")).toHaveValue("the new one is long");
    expect(screen.getByLabelText("Confirm new master password")).toHaveValue("the new one is long");
    expect(screen.getByRole("button", { name: "Change the master password" })).toBeEnabled();
  });

  it("can reveal the new master password only on request", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel api={buildApi()} />);
    await screen.findByRole("region", { name: "Master password" });

    expect(screen.getByLabelText("New master password")).toHaveAttribute("type", "password");
    await user.click(screen.getByRole("button", { name: "Show New master password" }));
    expect(screen.getByLabelText("New master password")).toHaveAttribute("type", "text");
  });

  it("updates the terminal preview from the appearance controls", async () => {
    const user = userEvent.setup();
    const { container } = render(<SettingsPanel api={buildApi()} />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    // The fields open once the loaded settings are in the draft.
    await waitFor(() => expect(within(region).getByLabelText("Colour scheme")).toBeEnabled());
    await user.selectOptions(within(region).getByLabelText("Colour scheme"), "nord");
    await user.type(within(region).getByLabelText("Font size"), "16");

    const preview = container.querySelector("[data-terminal-preview]");
    expect(preview).toHaveAttribute("data-term-palette", "nord");
    expect(preview).toHaveStyle({ fontSize: "16px" });
  });

  it("shows every terminal group as its own card on a phone, without tabs", async () => {
    vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })));
    render(<SettingsPanel api={buildApi()} page="Terminal" />);

    const region = await screen.findByRole("region", { name: "Terminal" });
    expect(within(region).queryByRole("tablist")).not.toBeInTheDocument();
    for (const heading of ["Startup", "Connection", "Display", "Behaviour"]) {
      expect(within(region).getByRole("region", { name: heading })).toBeVisible();
    }
    expect(within(region).getByLabelText(/^Starting directory/)).toBeInTheDocument();
    expect(within(region).getByRole("checkbox", { name: "Use WebGL rendering" })).toBeInTheDocument();
  });

  it("closes every open connection at once", async () => {
    const user = userEvent.setup();
    const closeAll = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsPanel
        api={buildApi()}
        terminalSessions={{
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
    // The confirmation names the sessions it ends, not only how many.
    const listed = within(screen.getByRole("dialog")).getByRole("list", { name: "Sessions that end" });
    expect(within(listed).getAllByRole("listitem").map((row) => row.textContent)).toEqual(["onelocalhost", "twolocalhost"]);
    await user.click(screen.getByRole("button", { name: "Close them all" }));

    expect(closeAll).toHaveBeenCalledTimes(1);
  });

  it("does not end every connection on the first press", async () => {
    const user = userEvent.setup();
    const closeAll = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsPanel
        api={buildApi()}
        terminalSessions={{
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
        terminalSessions={{
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
        terminalSessions={{ sessions: [], busy: false, closeAll: vi.fn() }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    expect(within(region).getByRole("button", { name: "Close every connection" })).toBeDisabled();
  });

  it("shows closing every connection as working until the sessions are closed", async () => {
    const user = userEvent.setup();
    let finishClosing: () => void = () => undefined;
    const closeAll = vi.fn(() => new Promise<void>((resolve) => {
      finishClosing = resolve;
    }));
    render(
      <SettingsPanel
        api={buildApi()}
        terminalSessions={{
          sessions: [
            { id: "a", title: "one", kind: "shell", forwards: [], exited: { code: 0, signal: "", at: "x" } },
          ] as never,
          busy: false,
          closeAll,
        }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    const closeEvery = within(region).getByRole("button", { name: "Close every connection" });
    await user.click(closeEvery);

    expect(closeEvery).toHaveAttribute("aria-busy", "true");
    await user.click(closeEvery);
    expect(closeAll).toHaveBeenCalledTimes(1);
    finishClosing();
    await waitFor(() => expect(closeEvery).not.toHaveAttribute("aria-busy"));
  });

  it("does not show closing as working while another part of the app is busy with sessions", async () => {
    render(
      <SettingsPanel
        api={buildApi()}
        terminalSessions={{
          sessions: [{ id: "a", title: "one", kind: "shell", forwards: [] }] as never,
          busy: true,
          closeAll: vi.fn(),
        }}
      />,
    );

    const region = await screen.findByRole("region", { name: "Open connections" });
    const closeEvery = within(region).getByRole("button", { name: "Close every connection" });
    expect(closeEvery).toBeDisabled();
    expect(closeEvery).not.toHaveAttribute("aria-busy");
  });

  it("shows the master password change working until the vault answers", async () => {
    const user = userEvent.setup();
    let answer: (value: { vault: ReturnType<typeof vaultStatus> }) => void = () => undefined;
    const changeMasterPassword = vi.fn(() => new Promise<{ vault: ReturnType<typeof vaultStatus> }>((resolve) => {
      answer = resolve;
    }));
    render(<SettingsPanel api={buildApi({ changeMasterPassword })} />);
    await fillMasterPassword(user);
    const change = screen.getByRole("button", { name: "Change the master password" });
    await user.click(change);

    expect(change).toHaveAttribute("aria-busy", "true");
    answer({ vault: vaultStatus() });
    expect(await screen.findByText(/remote snapshots were not rewritten/i)).toBeVisible();
    expect(change).not.toHaveAttribute("aria-busy");
  });
});


it("removes password protection after verifying the current password", async () => {
  const api = buildApi();
  render(<SettingsPanel api={api} page="Vault" />);
  await userEvent.type(await screen.findByLabelText("Current master password"), "old password");
  await userEvent.click(screen.getByLabelText("Use without a password"));
  expect(screen.queryByLabelText("New master password")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Change the master password" }));
  await waitFor(() => expect(api.changeMasterPassword).toHaveBeenCalledWith("old password", ""));
});

it("adds a four-character password to a passwordless vault", async () => {
  const api = buildApi({ passwordVault: vi.fn().mockResolvedValue(vaultStatus({ passwordless: true })) });
  render(<SettingsPanel api={api} page="Vault" />);
  await waitFor(() => expect(api.passwordVault).toHaveBeenCalled());
  expect(screen.queryByLabelText("Current master password")).not.toBeInTheDocument();
  await userEvent.type(screen.getByLabelText("New master password"), "1234");
  await userEvent.type(screen.getByLabelText("Confirm new master password"), "1234");
  await userEvent.click(screen.getByRole("button", { name: "Change the master password" }));
  await waitFor(() => expect(api.changeMasterPassword).toHaveBeenCalledWith("", "1234"));
  expect(await screen.findByLabelText("Current master password")).toBeInTheDocument();
});

it("hides the current password immediately after removing protection", async () => {
  const status = vaultStatus({ passwordless: true });
  const api = buildApi({ changeMasterPassword: vi.fn().mockResolvedValue({ vault: status }) });
  const onVaultChanged = vi.fn();
  render(<SettingsPanel api={api} page="Vault" onVaultChanged={onVaultChanged} />);
  await userEvent.type(await screen.findByLabelText("Current master password"), "old password");
  await userEvent.click(screen.getByLabelText("Use without a password"));
  await userEvent.click(screen.getByRole("button", { name: "Change the master password" }));
  await waitFor(() => expect(onVaultChanged).toHaveBeenCalledWith(status));
  expect(screen.queryByLabelText("Current master password")).not.toBeInTheDocument();
});

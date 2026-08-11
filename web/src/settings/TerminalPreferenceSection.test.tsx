import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { IntegrationsApi, TerminalOptionsResponse } from "../api/integrations";
import { TerminalPreferenceSection } from "./TerminalPreferenceSection";

const options: TerminalOptionsResponse = {
  selected: "terminal",
  terminals: [
    { id: "terminal", installed: true },
    { id: "iterm2", installed: true },
    { id: "kitty", installed: false },
    { id: "custom", installed: true },
  ],
  applications: [
    { name: "Warp", path: "/Applications/Warp.app" },
    { name: "Term", path: "/Applications/Term.app" },
  ],
};

function buildApi(overrides: Partial<IntegrationsApi> = {}) {
  return {
    terminalOptions: vi.fn().mockResolvedValue(options),
    setTerminalPreference: vi.fn().mockImplementation(async (request) => ({ ...options, ...request })),
    ...overrides,
  } as unknown as IntegrationsApi;
}

describe("TerminalPreferenceSection", () => {
  it("shows the saved global choice and explains unavailable terminals", async () => {
    render(<TerminalPreferenceSection api={buildApi()} />);

    const section = await screen.findByRole("region", { name: "Default connection application" });
    expect(within(section).getByLabelText("Open connections with")).toHaveValue("terminal");
    expect(within(section).getByRole("option", { name: "kitty — Not installed" })).toBeDisabled();
    expect(within(section).getByText(/applies to every connection/)).toBeInTheDocument();
  });

  it("saves an installed known terminal immediately", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<TerminalPreferenceSection api={api} />);

    await user.selectOptions(await screen.findByLabelText("Open connections with"), "iterm2");
    await waitFor(() => expect(api.setTerminalPreference).toHaveBeenCalledWith({ selected: "iterm2" }));
    expect(screen.getByLabelText("Open connections with")).toHaveValue("iterm2");
  });

  it("saves a detected custom application with argv words", async () => {
    const user = userEvent.setup();
    const api = buildApi();
    render(<TerminalPreferenceSection api={api} />);

    await user.selectOptions(await screen.findByLabelText("Open connections with"), "custom");
    await user.selectOptions(screen.getByLabelText("Application"), "/Applications/Warp.app");
    await user.type(screen.getByLabelText("Arguments"), "--new-window -e");
    await user.click(screen.getByRole("button", { name: "Save custom application" }));

    await waitFor(() => expect(api.setTerminalPreference).toHaveBeenCalledWith({
      selected: "custom",
      customTerminal: {
        application: "/Applications/Warp.app",
        arguments: ["--new-window", "-e"],
      },
    }));
  });

  it("restores the server value when saving fails", async () => {
    const user = userEvent.setup();
    const api = buildApi({
      setTerminalPreference: vi.fn().mockRejectedValue(new Error("write failed")),
    });
    render(<TerminalPreferenceSection api={api} />);

    await user.selectOptions(await screen.findByLabelText("Open connections with"), "iterm2");

    expect(await screen.findByText("The default connection application could not be saved.")).toBeInTheDocument();
    expect(screen.getByLabelText("Open connections with")).toHaveValue("terminal");
  });

  it("reports when terminal inventory cannot be loaded", async () => {
    render(<TerminalPreferenceSection api={buildApi({
      terminalOptions: vi.fn().mockRejectedValue(new Error("unavailable")),
    })} />);

    expect(await screen.findByText("The available connection applications could not be read.")).toBeInTheDocument();
  });

  it("does not resave a custom application that is no longer detected", async () => {
    render(<TerminalPreferenceSection api={buildApi({
      terminalOptions: vi.fn().mockResolvedValue({
        ...options,
        selected: "custom",
        customTerminal: { application: "/Applications/Gone.app", arguments: [] },
      }),
    })} />);

    expect(await screen.findByRole("button", { name: "Save custom application" })).toBeDisabled();
  });
});

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { IntegrationsApi } from "../api/integrations";
import { BackgroundPicker } from "./BackgroundPicker";

vi.mock("./backgroundImage", () => ({ useBackgroundImage: () => "" }));

describe("BackgroundPicker", () => {
  it("renames an uploaded image through the common dialog and follows the selected name", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const api = {
      terminalBackgrounds: vi.fn().mockResolvedValue({
        backgrounds: [{ name: "wall.png", bytes: 12, type: "image/png" }],
        usedBytes: 12,
        capacityBytes: 16 * 1024 * 1024,
        remainingBytes: 1024,
      }),
      addTerminalBackground: vi.fn(),
      setTerminalBackgroundCapacity: vi.fn(),
      renameTerminalBackground: vi.fn().mockResolvedValue({
        name: "night-sky.png", bytes: 12, type: "image/png",
      }),
      deleteTerminalBackground: vi.fn(),
    } satisfies Pick<IntegrationsApi, "terminalBackgrounds" | "addTerminalBackground" | "setTerminalBackgroundCapacity" | "renameTerminalBackground" | "deleteTerminalBackground">;

    render(
      <BackgroundPicker
        value="wall.png"
        onChange={onChange}
        tint={55}
        onTintChange={vi.fn()}
        unchosen="No image"
        api={api}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "Change" }));
    await user.click(screen.getByLabelText("Actions for wall.png"));
    await user.click(screen.getByRole("button", { name: "Rename" }));
    const dialog = screen.getByRole("dialog", { name: "Rename background image" });
    const input = within(dialog).getByRole("textbox", { name: "New file name" });
    await user.clear(input);
    await user.type(input, "Night Sky.jpg");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));

    expect(api.renameTerminalBackground).toHaveBeenCalledWith("wall.png", "Night Sky.jpg");
    expect(onChange).toHaveBeenCalledWith("night-sky.png");
  });
});

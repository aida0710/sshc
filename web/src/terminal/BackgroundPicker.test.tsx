import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { BackgroundPicker } from "./BackgroundPicker";
import { ApiError } from "../api/client";
import type { SettingsApi } from "../api/settings";

vi.mock("./backgroundImage", () => ({ useBackgroundImage: () => "" }));

const wallLibrary = {
  backgrounds: [{ name: "wall.png", bytes: 12, type: "image/png" }],
  usedBytes: 12,
  capacityBytes: 16 * 1024 * 1024,
  remainingBytes: 1024,
};

function libraryWithWall(overrides: Partial<Pick<SettingsApi, "deleteTerminalBackground">> = {}) {
  return {
    terminalBackgrounds: vi.fn().mockResolvedValue(wallLibrary),
    addTerminalBackground: vi.fn(),
    setTerminalBackgroundCapacity: vi.fn(),
    renameTerminalBackground: vi.fn(),
    deleteTerminalBackground: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

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
    } satisfies Pick<SettingsApi, "terminalBackgrounds" | "addTerminalBackground" | "setTerminalBackgroundCapacity" | "renameTerminalBackground" | "deleteTerminalBackground">;

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
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    const dialog = screen.getByRole("dialog", { name: "Rename background image" });
    const input = within(dialog).getByRole("textbox", { name: "New file name" });
    await user.clear(input);
    await user.type(input, "Night Sky.jpg");
    await user.click(within(dialog).getByRole("button", { name: "Rename" }));

    expect(api.renameTerminalBackground).toHaveBeenCalledWith("wall.png", "Night Sky.jpg");
    expect(onChange).toHaveBeenCalledWith("night-sky.png");
  });

  it("says the image is too large when the engine refuses the body at its entrance", async () => {
    const user = userEvent.setup();
    const api = {
      terminalBackgrounds: vi.fn().mockResolvedValue({
        backgrounds: [], usedBytes: 0, capacityBytes: 16 * 1024 * 1024, remainingBytes: 16 * 1024 * 1024,
      }),
      addTerminalBackground: vi.fn().mockRejectedValue(new ApiError("request_body_too_large", 413, null)),
      setTerminalBackgroundCapacity: vi.fn(),
      renameTerminalBackground: vi.fn(),
      deleteTerminalBackground: vi.fn(),
    } satisfies Pick<SettingsApi, "terminalBackgrounds" | "addTerminalBackground" | "setTerminalBackgroundCapacity" | "renameTerminalBackground" | "deleteTerminalBackground">;

    const { container } = render(
      <BackgroundPicker value="" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={api} />,
    );
    await user.click(await screen.findByRole("button", { name: "Change" }));
    const chooser = container.ownerDocument.querySelector<HTMLInputElement>('input[type="file"]');
    if (chooser === null) throw new Error("no file chooser");
    await user.upload(chooser, new File(["\x89PNG"], "photo.png", { type: "image/png" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("The image exceeds the maximum file size.");
  });

  it("opens an image's actions as a menu that Escape closes without closing the library", async () => {
    const user = userEvent.setup();
    render(<BackgroundPicker value="" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={libraryWithWall()} />);

    await user.click(screen.getByRole("button", { name: "Change" }));
    const actions = await screen.findByRole("button", { name: "Actions for wall.png" });
    await user.click(actions);
    expect(screen.getByRole("menuitem", { name: "Rename" })).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Background image library" })).toBeInTheDocument();
    expect(actions).toHaveFocus();
  });

  it("keeps one image's actions open at a time and closes them on a click elsewhere in the library", async () => {
    const user = userEvent.setup();
    const api = libraryWithWall();
    api.terminalBackgrounds.mockResolvedValue({
      ...wallLibrary,
      backgrounds: [...wallLibrary.backgrounds, { name: "sea.png", bytes: 12, type: "image/png" }],
    });
    render(<BackgroundPicker value="" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={api} />);

    await user.click(screen.getByRole("button", { name: "Change" }));
    await user.click(await screen.findByRole("button", { name: "Actions for wall.png" }));
    await user.click(screen.getByRole("button", { name: "Actions for sea.png" }));
    expect(screen.getAllByRole("menu")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Actions for sea.png" })).toHaveAttribute("aria-expanded", "true");

    fireEvent.pointerDown(screen.getByRole("heading", { name: "Background image library" }));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Background image library" })).toBeInTheDocument();
  });

  it("keeps the library open behind the rename dialog and returns focus to the image's actions", async () => {
    const user = userEvent.setup();
    render(<BackgroundPicker value="" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={libraryWithWall()} />);

    await user.click(screen.getByRole("button", { name: "Change" }));
    const actions = await screen.findByRole("button", { name: "Actions for wall.png" });
    await user.click(actions);
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    const dialog = screen.getByRole("dialog", { name: "Rename background image" });
    expect(screen.getByRole("dialog", { name: "Background image library" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Rename background image" })).not.toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Background image library" })).toBeInTheDocument();
    await vi.waitFor(() => expect(actions).toHaveFocus());
  });

  it("says in the delete confirmation that the image could not be deleted and keeps it open", async () => {
    const user = userEvent.setup();
    const api = libraryWithWall({ deleteTerminalBackground: vi.fn().mockRejectedValue(new Error("settings_failed")) });
    render(<BackgroundPicker value="wall.png" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={api} />);

    await user.click(await screen.findByRole("button", { name: "Change" }));
    await user.click(await screen.findByLabelText("Actions for wall.png"));
    await user.click(screen.getByRole("menuitem", { name: "Delete" }));
    const dialog = screen.getByRole("dialog", { name: "Delete background image?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The background image could not be deleted.");
    expect(screen.getByRole("dialog", { name: "Delete background image?" })).toBe(dialog);
  });

  it("says the saved images could not be read when the list cannot be reloaded after a delete", async () => {
    const user = userEvent.setup();
    const api = libraryWithWall();
    api.terminalBackgrounds.mockResolvedValueOnce(wallLibrary).mockRejectedValueOnce(new Error("settings_failed"));
    render(<BackgroundPicker value="" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={api} />);

    await user.click(await screen.findByRole("button", { name: "Change" }));
    await user.click(await screen.findByLabelText("Actions for wall.png"));
    await user.click(screen.getByRole("menuitem", { name: "Delete" }));
    await user.click(within(screen.getByRole("dialog", { name: "Delete background image?" })).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Saved images could not be read.")).toBeInTheDocument();
    expect(screen.queryByText("wall.png")).not.toBeInTheDocument();
  });

  it("says the saved images could not be read instead of calling the library empty", async () => {
    const user = userEvent.setup();
    const api = {
      terminalBackgrounds: vi.fn()
        .mockRejectedValueOnce(new Error("settings_failed"))
        .mockResolvedValue({ backgrounds: [{ name: "wall.png", bytes: 12, type: "image/png" }], usedBytes: 12, capacityBytes: 16 * 1024 * 1024, remainingBytes: 1024 }),
      addTerminalBackground: vi.fn(),
      setTerminalBackgroundCapacity: vi.fn(),
      renameTerminalBackground: vi.fn(),
      deleteTerminalBackground: vi.fn(),
    } satisfies Pick<SettingsApi, "terminalBackgrounds" | "addTerminalBackground" | "setTerminalBackgroundCapacity" | "renameTerminalBackground" | "deleteTerminalBackground">;
    render(<BackgroundPicker value="" onChange={vi.fn()} tint={55} onTintChange={vi.fn()} unchosen="No image" api={api} />);

    await user.click(screen.getByRole("button", { name: "Change" }));
    expect(await screen.findByText("Saved images could not be read.")).toBeInTheDocument();
    expect(screen.queryByText("No saved images")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("wall.png")).toBeInTheDocument();
  });
});

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { refreshPresetLibraries } from "../settings/presetLibrary";
import { fakeSettingsServer } from "../testing/settingsServer";
import { KeyBarConfig } from "./KeyBarConfig";
import type { KeyBarPreset } from "./keyBarPresets";

let presets: KeyBarPreset[];
let server: ReturnType<typeof fakeSettingsServer>;
beforeEach(async () => {
  presets = [];
  server = fakeSettingsServer({}, {
    read: async () => ({ schemaVersion: 10, presets: [{ id: "default" }], keyBarPresets: structuredClone(presets) }),
    mutate: async (path, init) => {
      expect(path).toBe("/api/v1/metadata/keybar");
      presets = (JSON.parse(init?.body as string) as { presets: KeyBarPreset[] }).presets;
      return {};
    },
  });
  await refreshPresetLibraries();
});
afterEach(() => vi.restoreAllMocks());

const shownRow = () => within(screen.getByRole("group", { name: "Keys always shown" }));
const moreRow = () => within(screen.getByRole("group", { name: "Behind 「…」" }));
const offered = (group: string) => within(screen.getByRole("group", { name: group }));
const savedKeys = () => presets[0]?.keys.map((key) => key.key ?? key.label);

describe("arranging the key bar", () => {
  it("keeps a change to the default layout in a key bar of one's own, which the preset then uses", async () => {
    render(<KeyBarConfig />);
    fireEvent.click(shownRow().getByRole("button", { name: "Tab" }));
    fireEvent.click(screen.getByRole("button", { name: "Later" }));

    await waitFor(() => expect(savedKeys()).toEqual(["DirectInput", "Escape", "Control", "Tab", "Ctrl+C", "ArrowLeft", "ArrowUp", "ArrowDown", "ArrowRight"]));
    expect(presets[0]?.name).toBe("My key order");
    expect(server.document().presets[0]?.interface?.keyBarPreset).toBe(presets[0]?.id);
    // The moved key stays selected, so it can be moved again.
    expect(shownRow().getByRole("button", { name: "Tab" })).toHaveAttribute("aria-pressed", "true");
  });

  it("offers the Claude Code key bar without saving it among the person's own", async () => {
    render(<KeyBarConfig />);
    const preset = screen.getByRole("combobox", { name: "Key bar preset" });
    expect(within(preset).getByRole("option", { name: "Claude Code" })).toBeInTheDocument();

    fireEvent.change(preset, { target: { value: "claude-code" } });

    await waitFor(() => expect(server.document().presets[0]?.interface?.keyBarPreset).toBe("claude-code"));
    expect(presets).toEqual([]);
    expect(await shownRow().findByRole("button", { name: "Shift+Tab" })).toBeInTheDocument();
    expect(preset).toHaveValue("claude-code");
  });

  it("keeps a change to the Claude Code key bar in a key bar of one's own, and leaves the built-in one as it was", async () => {
    render(<KeyBarConfig />);
    fireEvent.change(screen.getByRole("combobox", { name: "Key bar preset" }), { target: { value: "claude-code" } });
    await shownRow().findByRole("button", { name: "Shift+Tab" });
    fireEvent.click(shownRow().getByRole("button", { name: "Tab" }));
    fireEvent.click(screen.getByRole("button", { name: "Earlier" }));

    await waitFor(() => expect(savedKeys()?.slice(-2)).toEqual(["Tab", "@"]));
    expect(presets[0]?.name).toBe("My Claude Code");
    expect(presets[0]?.id).not.toBe("claude-code");
    expect(server.document().presets[0]?.interface?.keyBarPreset).toBe(presets[0]?.id);

    fireEvent.change(screen.getByRole("combobox", { name: "Key bar preset" }), { target: { value: "claude-code" } });
    await waitFor(() => expect(server.document().presets[0]?.interface?.keyBarPreset).toBe("claude-code"));
    const builtInRow = await shownRow().findAllByRole("button");
    expect(builtInRow.at(-1)).toHaveAccessibleName("Tab");
  });

  it("offers Shift+Tab to place", async () => {
    render(<KeyBarConfig />);
    fireEvent.click(offered("Input and modifiers").getByRole("button", { name: "Shift+Tab" }));

    await waitFor(() => expect(savedKeys()).toContain("Shift+Tab"));
    expect(offered("Input and modifiers").getByRole("button", { name: "Shift+Tab" })).toBeDisabled();
  });

  it("puts a key on offer after the selected key, and offers a named key only while it is not in the bar", async () => {
    render(<KeyBarConfig />);
    fireEvent.click(shownRow().getByRole("button", { name: "Esc" }));
    fireEvent.click(offered("Function keys").getByRole("button", { name: "F5" }));

    await waitFor(() => expect(savedKeys()?.slice(1, 4)).toEqual(["Escape", "F5", "Tab"]));
    expect(offered("Function keys").getByRole("button", { name: "F5" })).toBeDisabled();
    expect(offered("Input and modifiers").getByRole("button", { name: "Esc" })).toBeDisabled();
  });

  it("moves a key behind 「…」 and back, and removes one", async () => {
    render(<KeyBarConfig />);
    fireEvent.click(shownRow().getByRole("button", { name: "Ctrl+C" }));
    fireEvent.click(screen.getByRole("button", { name: "Behind 「…」" }));
    await waitFor(() => expect(presets[0]?.moreKeys.at(-1)).toEqual({ key: "Ctrl+C" }));
    expect(moreRow().getByRole("button", { name: "Ctrl+C" })).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByRole("button", { name: "Always shown" }));
    await waitFor(() => expect(savedKeys()?.at(-1)).toBe("Ctrl+C"));

    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(savedKeys()).not.toContain("Ctrl+C"));
    expect(offered("Ctrl combinations").getByRole("button", { name: "Ctrl+C" })).toBeEnabled();
  });

  it("makes a key that types a command and presses Enter, and edits it", async () => {
    render(<KeyBarConfig />);
    fireEvent.click(screen.getByRole("button", { name: "Make a key" }));
    fireEvent.change(screen.getByLabelText("Text on the key"), { target: { value: "git st" } });
    fireEvent.change(screen.getByLabelText("What it sends"), { target: { value: "git status" } });
    fireEvent.click(screen.getByLabelText("Press Enter after sending"));
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(presets[0]?.keys.at(-1)).toEqual({ label: "git st", text: "git status", enter: true }));
    expect(shownRow().getByRole("button", { name: "git st" })).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByLabelText("What it sends"), { target: { value: "git status -sb" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(presets[0]?.keys.at(-1)).toEqual({ label: "git st", text: "git status -sb", enter: true }));
  });

  it("refuses a key that would send a control character other than a tab or a line break", () => {
    render(<KeyBarConfig />);
    fireEvent.click(screen.getByRole("button", { name: "Make a key" }));
    fireEvent.change(screen.getByLabelText("Text on the key"), { target: { value: "up" } });
    fireEvent.change(screen.getByLabelText("What it sends"), { target: { value: "\x1b[A" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Only tabs and line breaks");
    expect(presets).toEqual([]);
  });
});

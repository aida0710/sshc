import { describe, expect, it, vi } from "vitest";
import { defaultBindings, shortcutActions } from "../keyconfig/bindings";
import { en } from "../i18n/messages/en";
import type { Translate } from "../i18n/context";
import { paletteActionCommands } from "./paletteActionCommands";

const t: Translate = (key) => en[key];
const everyAppAction = Object.fromEntries(shortcutActions.map((action) => [action, vi.fn()]));

describe("paletteActionCommands", () => {
  it("lists every shortcut action but the one that opens the palette and SFTP, which it leads with, in the settings' order", () => {
    const commands = paletteActionCommands({ t, bindings: defaultBindings, appActions: everyAppAction, hasSession: true, runOnTerminal: vi.fn() });
    expect(commands.map((command) => command.id)).toEqual(
      shortcutActions.filter((action) => action !== "palette" && action !== "sftp").map((action) => `action:${action}`),
    );
  });

  it("names an action as the settings do and gives it the keys assigned to it", () => {
    const [search] = paletteActionCommands({ t, bindings: defaultBindings, appActions: {}, hasSession: true, runOnTerminal: vi.fn() });
    expect(search).toMatchObject({ label: "Terminal search", keys: ["Ctrl+F", "Meta+F"] });
  });

  it("leaves out the actions that cannot act now, and the terminal's own with no session open", () => {
    const home = vi.fn();
    const commands = paletteActionCommands({ t, bindings: defaultBindings, appActions: { home }, hasSession: false, runOnTerminal: vi.fn() });
    expect(commands.map((command) => command.id)).toEqual(["action:home"]);
    void commands[0]?.run();
    expect(home).toHaveBeenCalledOnce();
  });

  it("hands an action of a terminal to the one that runs it on the terminal shown", () => {
    const runOnTerminal = vi.fn();
    const commands = paletteActionCommands({ t, bindings: defaultBindings, appActions: {}, hasSession: true, runOnTerminal });
    void commands.find((command) => command.id === "action:copy")?.run();
    expect(runOnTerminal).toHaveBeenCalledWith("copy");
  });
});

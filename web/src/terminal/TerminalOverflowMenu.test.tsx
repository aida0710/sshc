import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { presetInterfaceOf } from "../settings/presetInterface";
import { currentSettings } from "../settings/settingsStore";
import { fakeSettingsServer } from "../testing/settingsServer";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";

function renderMenu(onClose = () => undefined) {
  render(
    <LanguageProvider>
      <TerminalOverflowMenu osc52Enabled={false} onCopyContext={() => undefined} onCopyWithColors={() => undefined} onSaveScrollback={() => undefined} onToggleOsc52={() => undefined} onClose={onClose} />
    </LanguageProvider>,
  );
}

beforeEach(() => {
  fakeSettingsServer();
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "wakeLock");
  vi.restoreAllMocks();
});

describe("TerminalOverflowMenu", () => {
  it("groups secondary terminal actions and closes after invoking one", async () => {
    const copy = vi.fn();
    const close = vi.fn();
    render(
      <LanguageProvider>
        <TerminalOverflowMenu osc52Enabled={false} onQuickCommands={() => undefined} onCopyContext={copy} onCopyWithColors={() => undefined} onSaveScrollback={() => undefined} onToggleOsc52={() => undefined} onClose={close} />
      </LanguageProvider>,
    );

    expect(screen.getByRole("menuitemcheckbox", { name: /OSC 52/ })).toHaveAttribute("aria-checked", "false");
    await userEvent.click(screen.getByRole("menuitem", { name: "Copy recent terminal context" }));
    expect(copy).toHaveBeenCalledOnce();
    expect(close).toHaveBeenCalledOnce();
  });

  it("takes the terminal out of its split when offered, and offers it only then", async () => {
    const remove = vi.fn();
    const close = vi.fn();
    const { unmount } = render(
      <LanguageProvider>
        <TerminalOverflowMenu osc52Enabled={false} onRemoveFromSplit={remove} onCopyContext={() => undefined} onCopyWithColors={() => undefined} onSaveScrollback={() => undefined} onToggleOsc52={() => undefined} onClose={close} />
      </LanguageProvider>,
    );
    await userEvent.click(screen.getByRole("menuitem", { name: "Remove from split view" }));
    expect(remove).toHaveBeenCalledOnce();
    expect(close).toHaveBeenCalledOnce();
    unmount();

    renderMenu();
    expect(screen.queryByRole("menuitem", { name: "Remove from split view" })).not.toBeInTheDocument();
  });

  it("saves the scrollback to a file from the menu", async () => {
    const save = vi.fn();
    render(
      <LanguageProvider>
        <TerminalOverflowMenu osc52Enabled={false} onCopyContext={() => undefined} onCopyWithColors={() => undefined} onSaveScrollback={save} onToggleOsc52={() => undefined} onClose={() => undefined} />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("menuitem", { name: "Save scrollback to a file" }));

    expect(save).toHaveBeenCalledOnce();
  });

  it("copies the selection with its colors from the menu", async () => {
    const copy = vi.fn();
    render(
      <LanguageProvider>
        <TerminalOverflowMenu osc52Enabled={false} onCopyContext={() => undefined} onCopyWithColors={copy} onSaveScrollback={() => undefined} onToggleOsc52={() => undefined} onClose={() => undefined} />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("menuitem", { name: "Copy with colors" }));

    expect(copy).toHaveBeenCalledOnce();
  });

  it("treats its trigger as part of the layer and restores focus on Escape", async () => {
    const close = vi.fn();
    function Fixture() {
      const trigger = useRef<HTMLButtonElement>(null);
      return (
        <LanguageProvider>
          <button ref={trigger} type="button">More</button>
          <TerminalOverflowMenu
            triggerRef={trigger}
            osc52Enabled={false}
            onQuickCommands={() => undefined}
            onCopyContext={() => undefined} onCopyWithColors={() => undefined} onSaveScrollback={() => undefined}
            onToggleOsc52={() => undefined}
            onClose={close}
          />
        </LanguageProvider>
      );
    }
    render(<Fixture />);
    const trigger = screen.getByRole("button", { name: "More" });

    await userEvent.click(trigger);
    expect(close).not.toHaveBeenCalled();
    await userEvent.keyboard("{Escape}");

    expect(close).toHaveBeenCalledOnce();
    expect(trigger).toHaveFocus();
  });

  it("keeps the screen on for every terminal from the checkbox", async () => {
    Object.defineProperty(navigator, "wakeLock", { configurable: true, value: { request: vi.fn() } });
    const close = vi.fn();
    renderMenu(close);

    const item = screen.getByRole("menuitemcheckbox", { name: "Keep screen on" });
    expect(item).toHaveAttribute("aria-checked", "false");
    await userEvent.click(item);

    expect(presetInterfaceOf(currentSettings()).keepScreenAwake).toBe(true);
    expect(close).toHaveBeenCalledOnce();
  });

  it("does not offer to keep the screen on where the browser cannot", () => {
    renderMenu();

    expect(screen.queryByRole("menuitemcheckbox", { name: "Keep screen on" })).toBeNull();
  });

  it("fixes touch or mouse for every terminal once detection is turned off", async () => {
    const close = vi.fn();
    renderMenu(close);
    const detect = screen.getByRole("menuitemcheckbox", { name: "Detect touch automatically" });
    expect(detect).toHaveAttribute("aria-checked", "true");
    expect(screen.queryByRole("menuitemradio")).toBeNull();

    await userEvent.click(detect);
    // The menu stays open so that the fixed input can be picked; jsdom has no touch screen, so mouse is kept.
    expect(close).not.toHaveBeenCalled();
    expect(presetInterfaceOf(currentSettings()).touchMode).toBe("mouse");
    expect(screen.getByRole("menuitemradio", { name: "Use by mouse" })).toHaveAttribute("aria-checked", "true");

    await userEvent.click(screen.getByRole("menuitemradio", { name: "Use by touch" }));
    expect(presetInterfaceOf(currentSettings()).touchMode).toBe("touch");
    expect(close).toHaveBeenCalledOnce();
  });
});

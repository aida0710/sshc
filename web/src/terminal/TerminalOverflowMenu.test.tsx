import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";

describe("TerminalOverflowMenu", () => {
  it("groups secondary terminal actions and closes after invoking one", async () => {
    const copy = vi.fn();
    const close = vi.fn();
    render(
      <LanguageProvider>
        <TerminalOverflowMenu osc52Enabled={false} onQuickCommands={() => undefined} onCopyContext={copy} onToggleOsc52={() => undefined} onClose={close} />
      </LanguageProvider>,
    );

    expect(screen.getByRole("menuitemcheckbox", { name: /OSC 52/ })).toHaveAttribute("aria-checked", "false");
    await userEvent.click(screen.getByRole("menuitem", { name: "Copy recent terminal context" }));
    expect(copy).toHaveBeenCalledOnce();
    expect(close).toHaveBeenCalledOnce();
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
            onCopyContext={() => undefined}
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
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
});

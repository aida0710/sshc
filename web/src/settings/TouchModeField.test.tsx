import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { loadTouchMode, touchFirstQuery } from "../terminal/touchMode";
import { TouchModeField } from "./TouchModeField";

afterEach(() => {
  window.localStorage.clear();
  vi.unstubAllGlobals();
});

describe("TouchModeField", () => {
  it("detects touch until turned off, then fixes the input picked", async () => {
    vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
      matches: query === touchFirstQuery, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    })));
    render(<LanguageProvider><TouchModeField /></LanguageProvider>);
    const detect = screen.getByRole("checkbox", { name: "Detect automatically" });
    const fixed = screen.getByRole("combobox", { name: "Fixed input" });
    expect(detect).toBeChecked();
    expect(fixed).toBeDisabled();
    expect(fixed).toHaveValue("touch");

    await userEvent.click(detect);
    // Turning detection off keeps what it picked until another input is chosen.
    expect(loadTouchMode()).toBe("touch");
    expect(fixed).toBeEnabled();

    await userEvent.selectOptions(fixed, "mouse");
    expect(loadTouchMode()).toBe("mouse");

    await userEvent.click(detect);
    expect(loadTouchMode()).toBe("auto");
  });
});

import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { HostPickerDialog } from "./HostPickerDialog";

// phoneScreen makes every media query match, the way a phone answers mobileViewportQuery.
function phoneScreen(matches: boolean) {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    matches,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })));
}

function openPicker(initialFocus?: "search" | "close") {
  render(
    <LanguageProvider initial="en">
      <HostPickerDialog
        open
        heading="New session"
        aliases={["web-01"]}
        loadRecent={() => Promise.resolve({ connections: [] })}
        {...(initialFocus === undefined ? {} : { initialFocus })}
        onChoose={vi.fn()}
        onClose={vi.fn()}
      />
    </LanguageProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("HostPickerDialog", () => {
  it("focuses the search when it opens on a PC", async () => {
    phoneScreen(false);
    openPicker();
    await waitFor(() => expect(screen.getByRole("searchbox", { name: "Search connections" })).toHaveFocus());
  });

  it("focuses Close on a phone, so the keyboard does not cover the list", async () => {
    phoneScreen(true);
    openPicker();
    await waitFor(() => expect(screen.getByRole("button", { name: "Close host picker" })).toHaveFocus());
  });

  it("follows the focus the caller asks for", async () => {
    phoneScreen(true);
    openPicker("search");
    await waitFor(() => expect(screen.getByRole("searchbox", { name: "Search connections" })).toHaveFocus());
  });
});

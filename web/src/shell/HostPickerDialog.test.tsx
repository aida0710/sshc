import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { HostPickerDialog } from "./HostPickerDialog";
import type { DirectDestinationActions } from "./useDirectDestinationActions";

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

function openPickerWith(direct?: DirectDestinationActions) {
  render(
    <LanguageProvider initial="en">
      <HostPickerDialog
        open
        heading="New session"
        aliases={["web-01"]}
        loadRecent={() => Promise.resolve({ connections: [] })}
        direct={direct}
        onChoose={vi.fn()}
        onClose={vi.fn()}
      />
    </LanguageProvider>,
  );
}

function directActions(overrides: Partial<DirectDestinationActions> = {}): DirectDestinationActions {
  return { connect: vi.fn(), saveAndConnect: vi.fn(), aliasFor: () => "10-0-0-5", ...overrides };
}

const search = () => screen.getByRole("searchbox", { name: "Search connections" });

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

  it("offers to connect to what was typed, or to save it first", async () => {
    phoneScreen(false);
    const direct = directActions();
    openPickerWith(direct);
    await userEvent.type(search(), "deploy@10.0.0.5:2222");

    await userEvent.click(screen.getByRole("button", { name: /^Connect/ }));
    expect(direct.connect).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ text: "deploy@10.0.0.5:2222", user: "deploy", host: "10.0.0.5", port: 2222 }));

    await userEvent.type(search(), "deploy@10.0.0.5");
    expect(screen.getByRole("button", { name: /Connection name: 10-0-0-5/ })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /^Save as a connection and connect/ }));
    expect(direct.saveAndConnect).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ text: "deploy@10.0.0.5" }));
  });

  it("connects with Enter when a destination was typed, and does nothing with Enter on a search", async () => {
    phoneScreen(false);
    const direct = directActions();
    openPickerWith(direct);
    await userEvent.type(search(), "web{Enter}");
    expect(direct.connect).not.toHaveBeenCalled();
    await userEvent.clear(search());
    await userEvent.type(search(), "deploy@10.0.0.5{Enter}");
    expect(direct.connect).toHaveBeenCalledOnce();
  });

  it("offers only to connect where a saved connection could not be opened", async () => {
    phoneScreen(false);
    openPickerWith(directActions({ saveAndConnect: undefined }));
    await userEvent.type(search(), "deploy@10.0.0.5");
    expect(screen.getByRole("button", { name: /^Connect/ })).toBeVisible();
    expect(screen.queryByRole("button", { name: /^Save as a connection/ })).toBeNull();
  });

  it("offers nothing for a search, or where the picker only finds connections", async () => {
    phoneScreen(false);
    openPickerWith(directActions());
    await userEvent.type(search(), "nothing-like-it");
    expect(screen.queryByRole("button", { name: /^Connect/ })).toBeNull();
    expect(screen.getByText("No host matches this search.")).toBeVisible();
  });

  it("keeps a picker for files to the connections that exist", async () => {
    phoneScreen(false);
    openPickerWith();
    await userEvent.type(search(), "deploy@10.0.0.5");
    expect(screen.queryByRole("button", { name: /^Connect/ })).toBeNull();
    expect(screen.getByText("No host matches this search.")).toBeVisible();
  });
});

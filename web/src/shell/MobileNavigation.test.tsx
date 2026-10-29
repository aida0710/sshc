import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import type { Section } from "../routing/sectionRoute";
import { MobileNavigation } from "./MobileNavigation";

function markedTab(section: Section | null): string[] {
  render(
    <LanguageProvider initial="en">
      <MobileNavigation section={section} onNavigate={vi.fn()} />
    </LanguageProvider>,
  );
  const bar = screen.getByRole("navigation", { name: "Quick navigation" });
  return within(bar).getAllByRole("link")
    .filter((link) => link.getAttribute("aria-current") === "page")
    .map((link) => link.textContent ?? "");
}

describe("MobileNavigation", () => {
  it("marks the tab of a section it has", () => {
    expect(markedTab("Connections")).toEqual(["Connections"]);
  });

  it("marks Menu for a page opened from Menu, such as SSH Keys or Settings", () => {
    expect(markedTab("Keys")).toEqual(["Menu"]);
  });

  it("marks no tab on a page that is not a section", () => {
    expect(markedTab(null)).toEqual([]);
  });
});

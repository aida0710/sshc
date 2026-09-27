import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DisclosureChevron } from "./DisclosureChevron";

describe("DisclosureChevron", () => {
  it("points right while closed and down while open", () => {
    const { container, rerender } = render(<DisclosureChevron expanded={false} />);
    const chevron = () => container.querySelector("[data-icon='chevronRight']");
    expect(chevron()).toHaveAttribute("aria-hidden", "true");
    expect(chevron()?.getAttribute("class")).not.toContain("rotate-90");

    rerender(<DisclosureChevron expanded />);
    expect(chevron()?.getAttribute("class")).toContain("rotate-90");
  });

  it("follows only its own details when it is not told whether it is open", () => {
    const { container } = render(<DisclosureChevron />);
    expect(container.querySelector("[data-icon='chevronRight']")?.getAttribute("class")).toContain("[details[open]>summary_&]:rotate-90");
  });
});

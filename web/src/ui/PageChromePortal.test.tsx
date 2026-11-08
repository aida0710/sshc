import { render, screen } from "@testing-library/react";
import { Activity } from "react";
import { describe, expect, it } from "vitest";
import { PageChromePortal } from "./PageChromePortal";

function Sections({ shown, slot }: { shown: "Home" | "Connections"; slot: HTMLElement }) {
  return (["Home", "Connections"] as const).map((section) => (
    <Activity key={section} mode={section === shown ? "visible" : "hidden"}>
      <PageChromePortal slot={slot}>
        <button type="button">{`${section} action`}</button>
      </PageChromePortal>
    </Activity>
  ));
}

describe("PageChromePortal", () => {
  it("shows in the top bar only the action of the section shown, and switches with it", () => {
    const slot = document.createElement("div");
    document.body.appendChild(slot);
    const { rerender } = render(<Sections shown="Home" slot={slot} />);
    expect(screen.getByRole("button", { name: "Home action" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Connections action" })).not.toBeInTheDocument();

    rerender(<Sections shown="Connections" slot={slot} />);
    expect(screen.getByRole("button", { name: "Connections action" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Home action", hidden: true })).not.toBeVisible();
    slot.remove();
  });
});

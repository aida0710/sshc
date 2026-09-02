import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PanelState } from "./PanelState";

describe("PanelState", () => {
  it("announces loading without reporting a failure", () => {
    render(<PanelState tone="loading" title="Loading" />);

    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  });

  it("announces failed panels as alerts", () => {
    render(<PanelState tone="failed" title="Could not load" detail="Try again" />);

    expect(screen.getByRole("alert")).toHaveTextContent("Could not loadTry again");
  });
});

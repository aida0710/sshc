import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InspectorPane, InspectorToggle } from "./Inspector";

describe("InspectorToggle", () => {
  it("says whether the pane is open", () => {
    render(<InspectorToggle label="Display and classification" open={false} attention={false} onToggle={vi.fn()} />);

    const button = screen.getByRole("button", { name: "Show Display and classification" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(button).toHaveAttribute("aria-controls", "inspector");
    expect(button).toHaveTextContent("Display and classification");
  });

  it("changes its name when open", () => {
    render(<InspectorToggle label="Display and classification" open attention={false} onToggle={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Hide Display and classification" })).toHaveAttribute("aria-expanded", "true");
  });

  it("says so when what is inside needs attention", () => {
    render(<InspectorToggle label="Display and classification" open={false} attention onToggle={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Show Display and classification Needs attention" })).toBeInTheDocument();
  });

  it("does not say so otherwise", () => {
    render(<InspectorToggle label="Display and classification" open={false} attention={false} onToggle={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Show Display and classification" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Needs attention/ })).toBeNull();
  });

  it("reports a click", async () => {
    const onToggle = vi.fn();
    const user = userEvent.setup();
    render(<InspectorToggle label="Display and classification" open={false} attention={false} onToggle={onToggle} />);

    await user.click(screen.getByRole("button", { name: "Show Display and classification" }));

    expect(onToggle).toHaveBeenCalledOnce();
  });
});

describe("InspectorPane", () => {
  it("is a labelled complementary region the toggle can address", () => {
    render(<InspectorPane label="Details">nothing yet</InspectorPane>);

    const pane = screen.getByRole("complementary", { name: "Details" });
    expect(pane).toHaveAttribute("id", "inspector");
    expect(pane).toHaveTextContent("nothing yet");
  });

  it("draws no segmented control", () => {
    render(<InspectorPane label="Details">nothing yet</InspectorPane>);

    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });
});

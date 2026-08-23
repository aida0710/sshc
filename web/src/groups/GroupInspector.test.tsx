import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { GroupInspector } from "./GroupInspector";
import type { GroupMetadata } from "../api/config";

function group(overrides: Partial<GroupMetadata> = {}): GroupMetadata {
  return { name: "company", ...overrides } as GroupMetadata;
}

describe("GroupInspector", () => {
  it("edits the three things that live only in metadata", async () => {
    const onUpdate = vi.fn();
    const user = userEvent.setup();
    render(<GroupInspector group={group()} members={[]} onUpdate={onUpdate} />);

    expect(screen.getByText(/staged until you choose Save groups/)).toBeInTheDocument();

    await user.type(screen.getByLabelText("Display order"), "3");
    expect(onUpdate).toHaveBeenLastCalledWith({ order: 3 });

    expect(screen.getByLabelText("Colour")).toBeInTheDocument();
    expect(screen.getByLabelText("Hide company from Connections")).toBeInTheDocument();
  });

  it("offers hiding for a group that holds no connections of its own", async () => {
    const onUpdate = vi.fn();
    const user = userEvent.setup();
    render(<GroupInspector group={group({ name: "company/eu" })} members={[]} onUpdate={onUpdate} />);

    const toggle = screen.getByLabelText("Hide company/eu from Connections");
    expect(toggle).toBeEnabled();

    await user.click(toggle);

    expect(onUpdate).toHaveBeenCalledWith({ hidden: true });
  });

  it("refuses hiding for a group that holds connections, and says why", () => {
    render(<GroupInspector group={group()} members={["build01"]} onUpdate={vi.fn()} />);

    expect(screen.getByLabelText("Hide company from Connections")).toBeDisabled();
    expect(screen.getByText(/contains direct connections/)).toBeInTheDocument();
  });

  it("offers no clear button until there is a colour to clear", async () => {
    const onUpdate = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(<GroupInspector group={group()} members={[]} onUpdate={onUpdate} />);

    expect(screen.queryByRole("button", { name: "Clear company colour" })).toBeNull();

    rerender(<GroupInspector group={group({ colour: "#f97316" })} members={[]} onUpdate={onUpdate} />);
    await user.click(screen.getByRole("button", { name: "Clear company colour" }));

    expect(onUpdate).toHaveBeenLastCalledWith({ colour: "" });
  });
});

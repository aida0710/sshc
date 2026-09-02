import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CheckboxField, Field } from "./form";

describe("Field", () => {
  it("uses the shared error presentation", () => {
    render(<Field label="Port" error="Invalid port"><input /></Field>);

    const input = screen.getByLabelText("Port");
    const alert = screen.getByRole("alert");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAttribute("aria-describedby", alert.id);
    expect(alert).toHaveTextContent("Invalid port");
  });
});

describe("CheckboxField", () => {
  it("keeps preference hints and changes in one shared control", async () => {
    const onChange = vi.fn();
    render(<CheckboxField label="Auto sync" hint="Checks every minute" checked={false} onChange={onChange} />);

    await userEvent.click(screen.getByRole("checkbox", { name: /Auto sync/ }));
    expect(onChange).toHaveBeenCalledWith(true);
    expect(screen.getByText("Checks every minute")).toBeVisible();
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PasswordField, PasswordInput } from "./PasswordField";

describe("PasswordInput", () => {
  it("hides the value until the shared reveal control is pressed", async () => {
    const user = userEvent.setup();
    render(
      <PasswordInput label="Passphrase" value="secret" onChange={vi.fn()} />,
    );

    const input = screen.getByLabelText("Passphrase");
    expect(input).toHaveAttribute("type", "password");

    await user.click(screen.getByRole("button", { name: /show passphrase/i }));
    expect(input).toHaveAttribute("type", "text");

    await user.click(screen.getByRole("button", { name: /hide passphrase/i }));
    expect(input).toHaveAttribute("type", "password");
  });

  it("disables both the input and reveal control together", () => {
    render(
      <PasswordInput
        label="Password"
        value="secret"
        onChange={vi.fn()}
        disabled
      />,
    );

    expect(screen.getByLabelText("Password")).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /show password/i }),
    ).toBeDisabled();
  });
});

describe("PasswordField", () => {
  it("keeps the field label and hint while using the shared input", () => {
    render(
      <PasswordField
        label="Master password"
        value="secret"
        onChange={vi.fn()}
        hint="Stored only in memory."
      />,
    );

    expect(screen.getByLabelText("Master password")).toHaveAttribute(
      "type",
      "password",
    );
    expect(screen.getByText("Stored only in memory.")).toBeInTheDocument();
  });
});

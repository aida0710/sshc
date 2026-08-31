import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InputDialog } from "./InputDialog";

describe("InputDialog", () => {
  it("shows validation in the application and submits a trimmed value", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(
      <InputDialog
        id="name-heading"
        heading="Rename"
        label="New name"
        submitLabel="Save"
        cancelLabel="Cancel"
        validate={(value) => value === "" ? "Enter a name." : value.includes("/") ? "Do not use a slash." : ""}
        onSubmit={submit}
        onCancel={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a name.");
    await user.type(screen.getByRole("textbox", { name: "New name" }), "  valid  ");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(submit).toHaveBeenCalledWith("valid");
  });
});

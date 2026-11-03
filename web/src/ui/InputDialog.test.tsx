import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { InputDialog } from "./InputDialog";
import { asciiTextEntry } from "./softKeyboard";

afterEach(() => {
  vi.unstubAllGlobals();
});

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

  it("waits on the dialog while the submitted value is being applied, then lets the caller close it", async () => {
    let settle: () => void = () => undefined;
    const submit = vi.fn(() => new Promise<void>((resolve) => { settle = resolve; }));
    const cancel = vi.fn();
    const user = userEvent.setup();
    render(
      <InputDialog id="name-heading" heading="Rename" label="New name" initialValue="old" submitLabel="Save" cancelLabel="Cancel" onSubmit={submit} onCancel={cancel} />,
    );

    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByRole("button", { name: "Save" })).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(cancel).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(submit).toHaveBeenCalledTimes(1);

    settle();
    await waitFor(() => expect(screen.getByRole("button", { name: "Save" })).not.toHaveAttribute("aria-busy"));
  });

  it("shows a failure inside the dialog and puts the cursor back in the field to try again", async () => {
    let fail: () => void = () => undefined;
    const submit = vi.fn(() => new Promise<void>((_, reject) => { fail = () => reject(new Error("taken")); }));
    const user = userEvent.setup();
    const view = render(
      <InputDialog id="name-heading" heading="Rename" label="New name" initialValue="old" submitLabel="Save" cancelLabel="Cancel" onSubmit={submit} onCancel={vi.fn()} />,
    );

    await user.click(screen.getByRole("button", { name: "Save" }));
    view.rerender(
      <InputDialog id="name-heading" heading="Rename" label="New name" initialValue="old" submitLabel="Save" cancelLabel="Cancel" onSubmit={submit} onCancel={vi.fn()} error="That name is taken." />,
    );
    fail();

    expect(await screen.findByRole("alert")).toHaveTextContent("That name is taken.");
    expect(screen.getByRole("dialog")).toHaveAccessibleDescription("That name is taken.");
    await waitFor(() => expect(screen.getByRole("textbox", { name: "New name" })).toHaveFocus());
  });

  it("opens and fails with the keyboard down on a phone, and the field takes a tap", async () => {
    vi.stubGlobal("matchMedia", vi.fn((query: string) => ({ matches: true, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
    let fail: () => void = () => undefined;
    const submit = vi.fn(() => new Promise<void>((_, reject) => { fail = () => reject(new Error("taken")); }));
    const user = userEvent.setup();
    const view = render(
      <InputDialog id="name-heading" heading="Rename" label="New name" initialValue="old" submitLabel="Save" cancelLabel="Cancel" onSubmit={submit} onCancel={vi.fn()} />,
    );
    expect(screen.getByRole("dialog")).toHaveFocus();

    await user.click(screen.getByRole("button", { name: "Save" }));
    view.rerender(
      <InputDialog id="name-heading" heading="Rename" label="New name" initialValue="old" submitLabel="Save" cancelLabel="Cancel" onSubmit={submit} onCancel={vi.fn()} error="That name is taken." />,
    );
    fail();

    expect(await screen.findByRole("alert")).toHaveTextContent("That name is taken.");
    await waitFor(() => expect(screen.getByRole("button", { name: "Save" })).not.toHaveAttribute("aria-busy"));
    expect(screen.getByRole("textbox", { name: "New name" })).not.toHaveFocus();

    await user.click(screen.getByRole("textbox", { name: "New name" }));
    expect(screen.getByRole("textbox", { name: "New name" })).toHaveFocus();
  });

  it("opens the value's field with the keyboard the caller asks for", () => {
    render(
      <InputDialog id="name-heading" heading="Rename" label="New name" submitLabel="Save" cancelLabel="Cancel" textEntry={asciiTextEntry} onSubmit={vi.fn()} onCancel={vi.fn()} />,
    );
    expect(screen.getByRole("textbox", { name: "New name" })).toHaveAttribute("inputmode", "email");
  });
});

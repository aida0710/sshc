import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FieldRow } from "./FieldRow";

describe("FieldRow", () => {
  it("names its control by the row's name and reads the hint as its description", () => {
    render(
      <FieldRow label="Port" hint="Empty uses this machine's fixed port.">
        <input defaultValue="54447" />
      </FieldRow>,
    );

    const port = screen.getByLabelText("Port");
    expect(port).toHaveValue("54447");
    expect(port).toHaveAccessibleName("Port");
    expect(port).toHaveAccessibleDescription("Empty uses this machine's fixed port.");
  });

  it("reads a warning as part of the description and marks the control invalid", () => {
    render(
      <FieldRow label="Port" hint="Inherited from config:3" warning="Use a number from 1 to 65535.">
        <input defaultValue="0" />
      </FieldRow>,
    );

    const port = screen.getByLabelText("Port");
    expect(port).toHaveAccessibleDescription("Inherited from config:3 Use a number from 1 to 65535.");
    expect(port).toHaveAttribute("aria-invalid", "true");
  });

  it("keeps the row's actions outside the label so a click on them stays on them", () => {
    render(
      <FieldRow label="User" actions={<button type="button">Use the inherited user</button>}>
        <input defaultValue="aida" />
      </FieldRow>,
    );

    expect(screen.getByRole("button", { name: "Use the inherited user" }).closest("label")).toBeNull();
    expect(screen.getByLabelText("User")).toHaveAccessibleName("User");
  });

  it("leaves a control made of several parts to name itself", () => {
    render(
      <FieldRow label="Passphrase" interactiveChildren>
        <input aria-label="Passphrase" />
        <button type="button">Show</button>
      </FieldRow>,
    );

    expect(screen.getByRole("button", { name: "Show" }).closest("label")).toBeNull();
  });

  it("lines an editor row up on the editor's own width rather than the window's", () => {
    render(
      <FieldRow label="Port" density="editor" hint="Empty uses 22.">
        <input defaultValue="2222" />
      </FieldRow>,
    );

    const port = screen.getByLabelText("Port");
    expect(port).toHaveAccessibleName("Port");
    expect(port).toHaveAccessibleDescription("Empty uses 22.");
    const row = port.closest("label")!.parentElement!;
    expect(row.className).toContain("@sm/editor:grid-cols-[132px_minmax(0,1fr)]");
    expect(row.className).not.toContain("lg:grid-cols");
  });
});

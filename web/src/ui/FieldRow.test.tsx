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

  it("leaves a control made of several parts to name itself", () => {
    render(
      <FieldRow label="Passphrase" interactiveChildren>
        <input aria-label="Passphrase" />
        <button type="button">Show</button>
      </FieldRow>,
    );

    expect(screen.getByRole("button", { name: "Show" }).closest("label")).toBeNull();
  });
});

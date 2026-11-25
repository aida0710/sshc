import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FieldProblems } from "./FieldProblems";

describe("FieldProblems", () => {
  it("names the field before what is wrong with it, under the fields", () => {
    render(<FieldProblems problems={[{ id: "name-problem", field: "Connection name", text: "Use letters and numbers." }]} />);

    expect(document.getElementById("name-problem")).toHaveTextContent("Connection name: Use letters and numbers.");
  });

  it("does not name the field twice when the sentence already starts with it", () => {
    render(<FieldProblems problems={[{ id: "port-problem", field: "Port", text: "Port must be a whole number from 1 to 65535." }]} />);

    expect(screen.getByText("Port must be a whole number from 1 to 65535.")).toBeInTheDocument();
  });

  it("shows nothing when no field has a problem", () => {
    const { container } = render(<FieldProblems problems={[{ id: "user-problem", field: "User", text: "" }]} />);

    expect(container).toBeEmptyDOMElement();
  });
});

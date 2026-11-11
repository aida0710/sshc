import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { QRCode } from "./QRCode";

describe("QRCode", () => {
  it("draws each dark module one unit square, inside a margin of four modules", () => {
    render(<QRCode rows={["10", "01"]} label="Sync settings" />);

    const code = screen.getByRole("img", { name: "Sync settings" });
    expect(code).toHaveAttribute("viewBox", "0 0 10 10");
    expect(code.querySelector("path")).toHaveAttribute("d", "M4 4h1v1h-1zM5 5h1v1h-1z");
  });
});

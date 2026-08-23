import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CrashBoundary } from "./CrashBoundary";

function Broken(): never {
  throw new TypeError("undefined is not a function");
}

describe("CrashBoundary", () => {
  it("names the failure instead of leaving an empty page", () => {
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <CrashBoundary>
        <Broken />
      </CrashBoundary>,
    );
    expect(screen.getByText(/TypeError: undefined is not a function/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
    quiet.mockRestore();
  });

  it("stays out of the way when nothing throws", () => {
    render(
      <CrashBoundary>
        <p>the application</p>
      </CrashBoundary>,
    );
    expect(screen.getByText("the application")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reload" })).toBeNull();
  });
});

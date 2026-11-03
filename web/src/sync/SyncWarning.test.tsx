import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { SyncWarning } from "./SyncWarning";

describe("SyncWarning", () => {
  it("says in one line that private keys may be sent, and opens the whole of it on request", async () => {
    render(<SyncWarning />);

    expect(screen.getByText("Private keys may be included.")).toBeVisible();
    expect(screen.queryByText(/does not match the exclusion rules|do not match the exclusion rules/u)).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.getByText(/do not match the exclusion rules/u)).toBeVisible();
    expect(screen.getByRole("button", { name: "Hide details" })).toHaveAttribute("aria-expanded", "true");
  });
});

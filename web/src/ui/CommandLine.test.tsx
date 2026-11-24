import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CommandLine } from "./CommandLine";

describe("CommandLine", () => {
  it("copies exactly the command it shows", async () => {
    const user = userEvent.setup();
    render(<CommandLine command="sshc service install" note="Start it as a service" />);

    await user.click(screen.getByRole("button", { name: "Copy sshc service install" }));

    expect(await navigator.clipboard.readText()).toBe("sshc service install");
    expect(screen.getByRole("button", { name: "Copied." })).toBeInTheDocument();
  });

  it("says the browser refused the copy in place of the note", async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    render(<CommandLine command="sshc open" note="Start it without a service" />);

    await user.click(screen.getByRole("button", { name: "Copy sshc open" }));

    expect(await screen.findAllByText("The browser refused to write to the clipboard.")).not.toHaveLength(0);
    expect(screen.queryByText("Start it without a service")).not.toBeInTheDocument();
  });
});

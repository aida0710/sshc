import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CopyButton } from "./CopyButton";
import { copiedVisibleMs } from "./useClipboardCopy";

describe("CopyButton", () => {
  it("writes exactly the value it was given", async () => {
    const user = userEvent.setup();
    render(<CopyButton value="ssh -- bastion" label="copy.remoteCommand" />);

    await user.click(screen.getByRole("button", { name: "Copy remote command" }));

    expect(await navigator.clipboard.readText()).toBe("ssh -- bastion");
    expect(screen.getByText("Copied.")).toBeInTheDocument();
  });

  it("says the write was refused rather than claiming it succeeded", async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    render(<CopyButton value="ssh -- bastion" label="copy.remoteCommand" />);

    await user.click(screen.getByRole("button", { name: "Copy remote command" }));

    expect(await screen.findByText(/refused to write to the clipboard/)).toBeInTheDocument();
    expect(screen.queryByText("Copied.")).not.toBeInTheDocument();
  });

  it("stops claiming a copy once the value has changed underneath it", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<CopyButton value="first" label="copy.remoteCommand" />);

    await user.click(screen.getByRole("button", { name: "Copy remote command" }));
    expect(screen.getByText("Copied.")).toBeInTheDocument();

    rerender(<CopyButton value="second" label="copy.remoteCommand" />);

    expect(screen.queryByText("Copied.")).not.toBeInTheDocument();
    expect(await navigator.clipboard.readText()).toBe("first");
  });

  it("shows the copy on the pressed button for a moment, then offers it again", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(<CopyButton value="ssh -- bastion" label="copy.remoteCommand" />);

      await user.click(screen.getByRole("button", { name: "Copy remote command" }));
      expect(await screen.findByRole("button", { name: "Copied" })).toBeInTheDocument();

      await act(async () => { vi.advanceTimersByTime(copiedVisibleMs); });
      expect(screen.getByRole("button", { name: "Copy remote command" })).toBeInTheDocument();
      expect(screen.queryByText("Copied.")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("names what it copies, so two buttons on one screen are distinguishable", () => {
    render(
      <>
        <CopyButton value="a" label="copy.keyLine" />
        <CopyButton value="b" label="copy.remoteCommand" />
      </>,
    );

    expect(screen.getByRole("button", { name: "Copy key line" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy remote command" })).toBeInTheDocument();
  });
});

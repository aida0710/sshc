import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { TerminalLinkPopover } from "./TerminalLinkPopover";

const writeText = vi.hoisted(() => vi.fn());
vi.mock("../ui/clipboard", () => ({ clipboard: { readText: vi.fn(), writeText } }));

describe("TerminalLinkPopover", () => {
  it("passes a remote file action to SFTP", async () => {
    const onRemotePath = vi.fn();
    const onClose = vi.fn();
    render(
      <LanguageProvider>
        <TerminalLinkPopover
          selection={{
            link: { kind: "remote-path", text: "/var/log/app.log", target: "/var/log/app.log", start: 0, end: 16 },
            x: 20,
            y: 30,
          }}
          onRemotePath={onRemotePath}
          onClose={onClose}
        />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Edit with SFTP" }));

    expect(onRemotePath).toHaveBeenCalledWith("/var/log/app.log", "edit");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("opens an HTTP link in a new isolated browser tab", async () => {
    const opened = { opener: window, close: vi.fn() };
    const open = vi.spyOn(window, "open").mockReturnValue(opened as unknown as Window);
    render(
      <LanguageProvider>
        <TerminalLinkPopover
          selection={{
            link: { kind: "url", text: "https://example.com/docs", target: "https://example.com/docs", start: 0, end: 24 },
            x: 20,
            y: 30,
          }}
          onClose={() => undefined}
        />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Open in browser" }));

    expect(open).toHaveBeenCalledWith("https://example.com/docs", "_blank", "noopener,noreferrer");
    expect(opened.opener).toBeNull();
    open.mockRestore();
  });

  it("copies the hidden destination rather than the visible OSC 8 label", async () => {
    writeText.mockResolvedValue(undefined);
    render(
      <LanguageProvider>
        <TerminalLinkPopover
          selection={{
            link: { kind: "url", text: "documentation", target: "https://example.com/actual", start: 0, end: 13 },
            x: 20,
            y: 30,
          }}
          onClose={() => undefined}
        />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Copy link" }));
    expect(writeText).toHaveBeenCalledWith("https://example.com/actual");
  });

  it("moves focus to the first action and returns it to the terminal when closed with Escape", async () => {
    const user = userEvent.setup();
    const terminalInput = document.createElement("textarea");
    document.body.append(terminalInput);
    terminalInput.focus();
    function Harness() {
      const [open, setOpen] = useState(true);
      return open ? (
        <TerminalLinkPopover
          selection={{
            link: { kind: "url", text: "https://example.com/docs", target: "https://example.com/docs", start: 0, end: 24 },
            x: 20,
            y: 30,
          }}
          onClose={() => setOpen(false)}
        />
      ) : null;
    }
    render(<LanguageProvider><Harness /></LanguageProvider>);

    expect(screen.getByRole("button", { name: "Open in browser" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "Copy link" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "Open in browser" })).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(terminalInput).toHaveFocus());
    terminalInput.remove();
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { TerminalLinkPopover } from "./TerminalLinkPopover";

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
});

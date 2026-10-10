import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/terminalSessions";
import { LanguageProvider } from "../i18n/context";
import { TerminalPortForwards } from "./TerminalPortForwards";

const session: TerminalSession = {
  id: "session-1",
  kind: "ssh",
  alias: "bastion",
  title: "bastion",
  startedAt: "2026-08-30T00:00:00Z",
  state: "connected",
  problem: "",
};

function listed(forwards: NonNullable<TerminalSession["forwards"]>) {
  return { sessions: [{ ...session, forwards }], maxSessions: 50 };
}

describe("TerminalPortForwards", () => {
  it("starts and saves a remote tunnel with a clearly identified SSH server port", async () => {
    const forward = { id: "pf-remote", kind: "remote", listen: "127.0.0.1:9080", to: "127.0.0.1:3000", problem: "", temporary: true };
    const api = { startTerminalForward: vi.fn().mockResolvedValue(listed([forward])), stopTerminalForward: vi.fn().mockResolvedValue(listed([])) };
    const saveApi = {
      overview: vi.fn().mockResolvedValue({ hosts: [{ identity: { path: "connections/work.conf", alias: "bastion" } }] }),
      host: vi.fn().mockResolvedValue({ file: { contents: "Host bastion\n" } }),
      save: vi.fn().mockResolvedValue({}),
    };
    render(<LanguageProvider><TerminalPortForwards session={session} api={api} saveApi={saveApi as never} onClose={vi.fn()} /></LanguageProvider>);
    await userEvent.selectOptions(screen.getByLabelText("Type"), "remote");
    await userEvent.type(screen.getByLabelText("SSH server port"), "9080");
    await userEvent.type(screen.getByLabelText("Destination"), "127.0.0.1:3000");
    await userEvent.click(screen.getByRole("checkbox", { name: /Save to this connection/ }));
    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(api.startTerminalForward).toHaveBeenCalledWith("session-1", { kind: "remote", listenPort: 9080, destination: "127.0.0.1:3000" });
    expect(saveApi.save).toHaveBeenCalledWith(expect.objectContaining({ fields: [{ action: "add", keyword: "RemoteForward", values: ["9080", "127.0.0.1:3000"] }] }));
    expect(screen.getByText("SSH server 127.0.0.1:9080 → engine 127.0.0.1:3000")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Stop" }));
    expect(api.stopTerminalForward).toHaveBeenCalledWith("session-1", "pf-remote");
  });

  it("explains when the SSH server denies the remote listener", async () => {
    const api = {
      startTerminalForward: vi.fn().mockRejectedValue(new ApiError("terminal_forward_bind_failed", 409, { code: "terminal_forward_bind_failed", message: "request rejected", reason: "remote_denied" })),
      stopTerminalForward: vi.fn(),
    };
    render(<LanguageProvider><TerminalPortForwards session={session} api={api} onClose={vi.fn()} /></LanguageProvider>);
    await userEvent.selectOptions(screen.getByLabelText("Type"), "remote");
    await userEvent.type(screen.getByLabelText("SSH server port"), "9080");
    await userEvent.type(screen.getByLabelText("Destination"), "127.0.0.1:3000");
    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The SSH server refused the listener.");
  });

  it("starts a temporary Local tunnel and can stop it without closing the terminal", async () => {
    const forward = {
      id: "pf-1", kind: "local", listen: "127.0.0.1:8080", to: "db.internal:5432",
      problem: "", temporary: true,
    };
    const api = {
      startTerminalForward: vi.fn().mockResolvedValue(listed([forward])),
      stopTerminalForward: vi.fn().mockResolvedValue(listed([])),
    };
    render(<LanguageProvider><TerminalPortForwards session={session} api={api} onClose={vi.fn()} /></LanguageProvider>);

    await userEvent.type(screen.getByLabelText("Local port"), "8080");
    await userEvent.type(screen.getByLabelText("Destination"), "db.internal:5432");
    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(api.startTerminalForward).toHaveBeenCalledWith("session-1", {
      kind: "local", listenPort: 8080, destination: "db.internal:5432",
    });
    expect(screen.getByText("127.0.0.1:8080 → db.internal:5432")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Stop" }));
    expect(api.stopTerminalForward).toHaveBeenCalledWith("session-1", "pf-1");
    expect(screen.getByText("No forwarding is active on this connection.")).toBeVisible();
  });

  it("surfaces the safe bind detail returned by the engine", async () => {
    const api = {
      startTerminalForward: vi.fn().mockRejectedValue(new ApiError("terminal_forward_bind_failed", 409, {
        code: "terminal_forward_bind_failed",
        message: "terminal_forward_bind_failed",
        detail: "listen tcp 127.0.0.1:8080: bind: address already in use",
      })),
      stopTerminalForward: vi.fn(),
    };
    render(<LanguageProvider><TerminalPortForwards session={session} api={api} onClose={vi.fn()} /></LanguageProvider>);
    await userEvent.type(screen.getByLabelText("Local port"), "8080");
    await userEvent.type(screen.getByLabelText("Destination"), "db.internal:5432");
    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("address already in use");
  });

  it("says a port already in use in words instead of the engine's error", async () => {
    const api = {
      startTerminalForward: vi.fn().mockRejectedValue(new ApiError("terminal_forward_bind_failed", 409, {
        code: "terminal_forward_bind_failed",
        message: "request rejected",
        reason: "address_in_use",
        detail: "listen tcp 127.0.0.1:8080: bind: address already in use",
      })),
      stopTerminalForward: vi.fn(),
    };
    render(<LanguageProvider><TerminalPortForwards session={session} api={api} onClose={vi.fn()} /></LanguageProvider>);
    await userEvent.type(screen.getByLabelText("Local port"), "8080");
    await userEvent.type(screen.getByLabelText("Destination"), "db.internal:5432");
    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The local port is already in use.");
    expect(screen.queryByText(/bind: address already in use/)).not.toBeInTheDocument();
  });

  it("says why a saved forwarding could not be opened in words", () => {
    const failed = { ...session, forwards: [{
      id: "", kind: "local", listen: "127.0.0.1:8080", to: "db.internal:5432",
      problem: "address_in_use", temporary: false,
    }] };
    render(<LanguageProvider><TerminalPortForwards session={failed} api={{ startTerminalForward: vi.fn(), stopTerminalForward: vi.fn() }} onClose={vi.fn()} /></LanguageProvider>);
    expect(screen.getByRole("alert")).toHaveTextContent("The local port is already in use.");
    expect(screen.queryByText("address_in_use")).not.toBeInTheDocument();
  });

  it("optionally writes the same forwarding to the selected connection", async () => {
    const forward = {
      id: "pf-1", kind: "dynamic", listen: "127.0.0.1:1080", to: "",
      problem: "", temporary: true,
    };
    const api = {
      startTerminalForward: vi.fn().mockResolvedValue(listed([forward])),
      stopTerminalForward: vi.fn(),
    };
    const saveApi = {
      overview: vi.fn().mockResolvedValue({ hosts: [{ identity: { path: "connections/work.conf", alias: "bastion" } }] }),
      host: vi.fn().mockResolvedValue({ file: { contents: "Host bastion\n" } }),
      save: vi.fn().mockResolvedValue({}),
    };
    render(<LanguageProvider><TerminalPortForwards session={session} api={api} saveApi={saveApi as never} onClose={vi.fn()} /></LanguageProvider>);
    await userEvent.selectOptions(screen.getByLabelText("Type"), "dynamic");
    expect(screen.queryByLabelText("Destination")).not.toBeInTheDocument();
    expect(screen.getByText("The application using this SOCKS proxy chooses the destination for each connection.")).toBeVisible();
    await userEvent.type(screen.getByLabelText("Local port"), "1080");
    await userEvent.click(screen.getByRole("checkbox", { name: /Save to this connection/ }));
    await userEvent.click(screen.getByRole("button", { name: "Start" }));

    expect(saveApi.save).toHaveBeenCalledWith({
      kind: "host_fields",
      path: "connections/work.conf",
      alias: "bastion",
      base: "Host bastion\n",
      fields: [{ action: "add", keyword: "DynamicForward", values: ["1080"] }],
    });
    expect(screen.getByText("The forwarding is active and was saved to the connection.")).toBeVisible();
  });
});

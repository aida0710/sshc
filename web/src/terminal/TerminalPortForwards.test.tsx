import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/integrations";
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

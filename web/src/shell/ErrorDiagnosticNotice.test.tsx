import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { ErrorDiagnosticNotice, diagnosticReport } from "./ErrorDiagnosticNotice";

describe("ErrorDiagnosticNotice", () => {
  it("renders a copyable report without a query or request body", async () => {
    const user = userEvent.setup();
    const close = vi.fn();
    render(
      <LanguageProvider>
        <ErrorDiagnosticNotice
          version="0.13.4"
          diagnostic={{
            code: "sftp_failed",
            status: 502,
            method: "post",
            path: "/api/v1/sftp/transfers",
            detail: "the remote server closed the connection",
          }}
          onClose={close}
        />
      </LanguageProvider>,
    );

    await user.click(screen.getByText("Show diagnostic details"));
    expect(screen.getByText(/Version: 0\.13\.4/)).toHaveTextContent(
      "Operation: POST /api/v1/sftp/transfers",
    );
    await user.click(screen.getByRole("button", { name: "Dismiss error" }));
    expect(close).toHaveBeenCalledTimes(1);
  });

  it("labels a failed network request without exposing an exception", () => {
    expect(diagnosticReport("", {
      code: "network_request_failed",
      status: 0,
      method: "GET",
      path: "/api/v1/sync/status",
    })).toBe([
      "Version: unknown",
      "Code: network_request_failed",
      "HTTP: network unavailable",
      "Operation: GET /api/v1/sync/status",
    ].join("\n"));
  });
});

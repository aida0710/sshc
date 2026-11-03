import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { LanguageProvider } from "../i18n/context";
import { SessionDestinationLine } from "./SessionDestinationLine";
import { SessionAddressesContext } from "./sessionDestination";

const session = (overrides: Partial<TerminalSession>): TerminalSession => ({
  id: "one", kind: "ssh", workspaceId: "main", alias: "web-01", title: "web-01", startedAt: "2026-11-03T08:00:00Z",
  state: "connected", problem: "", ...overrides,
});

function renderLine(shown: TerminalSession) {
  render(
    <LanguageProvider>
      <SessionAddressesContext.Provider value={new Map([["web-01", "deploy@198.51.100.11"]])}>
        <SessionDestinationLine session={shown} />
      </SessionAddressesContext.Provider>
    </LanguageProvider>,
  );
}

describe("the line under a session's name", () => {
  it("says the address when the name is the alias, instead of the alias again", () => {
    renderLine(session({}));
    expect(screen.getByText("deploy@198.51.100.11")).toBeVisible();
  });

  it("says the alias under a name of its own", () => {
    renderLine(session({ title: "web-01 logs" }));
    expect(screen.getByText("web-01")).toBeVisible();
  });

  it("keeps the alias when the address is not known, as on a run target", () => {
    renderLine(session({ alias: "db-01", title: "db-01" }));
    expect(screen.getByText("db-01")).toBeVisible();
  });

  it("does not take a local shell's name for an alias", () => {
    renderLine(session({ kind: "shell", alias: undefined as never, title: "web-01" }));
    expect(screen.getByText("localhost")).toBeVisible();
  });
});

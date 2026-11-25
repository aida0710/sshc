import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LanguageProvider } from "../../i18n/context";
import { HomeMachineSection } from "./HomeMachineSection";
import type { TerminalOverview } from "./useTerminalOverview";

const machine = { hostname: "atlas", os: "linux", cpuPercent: 12 };
const hours = [{ hour: "2026-11-20T10:00:00Z", connections: 3, aliases: ["bastion"] }];

function renderSection(overview: TerminalOverview, runTarget: string | null = null) {
  return render(<LanguageProvider initial="en"><HomeMachineSection overview={overview} runTarget={runTarget} /></LanguageProvider>);
}

describe("HomeMachineSection", () => {
  it("shows the machine and the activity of the engine new sessions open on", () => {
    renderSection({ machine, cpuHistory: [12], history: { hours, recent: [] } });
    expect(screen.getByLabelText("The machine new shells open on")).toHaveTextContent("atlas");
    expect(screen.getByRole("heading", { name: "Activity" })).toBeInTheDocument();
  });

  it("names the run target before its machine", () => {
    renderSection({ machine, cpuHistory: [], history: null }, "r100");
    expect(screen.getByLabelText("The machine new shells open on")).toHaveTextContent(/^r100/);
  });

  it("leaves out the activity until a connection has been opened, and everything while the engine gives nothing", () => {
    const { container, unmount } = renderSection({ machine, cpuHistory: [], history: { hours: [], recent: [] } });
    expect(screen.queryByRole("heading", { name: "Activity" })).toBeNull();
    expect(container).not.toBeEmptyDOMElement();
    unmount();
    expect(renderSection({ machine: null, cpuHistory: [], history: null }).container).toBeEmptyDOMElement();
  });
});

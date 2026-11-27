import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LanguageProvider } from "../../i18n/context";
import { HomeMachineSection } from "./HomeMachineSection";
import type { TerminalOverview } from "./useTerminalOverview";

const machine = { hostname: "atlas", os: "linux", cpuPercent: 12 };

function renderSection(overview: TerminalOverview, runTarget: string | null = null) {
  return render(<LanguageProvider initial="en"><HomeMachineSection overview={overview} runTarget={runTarget} /></LanguageProvider>);
}

describe("HomeMachineSection", () => {
  it("shows the machine of the engine new sessions open on", () => {
    renderSection({ machine, cpuHistory: [12] });
    expect(screen.getByLabelText("The machine new shells open on")).toHaveTextContent("atlas");
  });

  it("names the run target before its machine", () => {
    renderSection({ machine, cpuHistory: [] }, "r100");
    expect(screen.getByLabelText("The machine new shells open on")).toHaveTextContent(/^r100/);
  });

  it("shows nothing while the engine gives no machine", () => {
    expect(renderSection({ machine: null, cpuHistory: [] }).container).toBeEmptyDOMElement();
  });
});

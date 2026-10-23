import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EngineActionOutput } from "./EngineActionOutput";

// layOut gives the log the sizes jsdom does not compute: content taller than
// the box, so it can scroll.
function layOut(log: HTMLElement, scrollHeight: number) {
  Object.defineProperty(log, "clientHeight", { configurable: true, value: 100 });
  Object.defineProperty(log, "scrollHeight", { configurable: true, value: scrollHeight });
}

describe("EngineActionOutput", () => {
  it("shows nothing until the host's sshc writes something", () => {
    render(<EngineActionOutput output="" />);
    expect(screen.queryByRole("log")).not.toBeInTheDocument();
  });

  it("keeps the newest line in view until the person scrolls up, and again once they scroll back down", () => {
    const { rerender } = render(<EngineActionOutput output="one\n" />);
    const log = screen.getByRole("log", { name: "Output of sshc on that host" });
    layOut(log, 300);
    rerender(<EngineActionOutput output={"one\ntwo\n"} />);
    expect(log.scrollTop).toBe(300);

    log.scrollTop = 50;
    fireEvent.scroll(log);
    layOut(log, 400);
    rerender(<EngineActionOutput output={"one\ntwo\nthree\n"} />);
    expect(log.scrollTop).toBe(50);

    log.scrollTop = 300;
    fireEvent.scroll(log);
    layOut(log, 500);
    rerender(<EngineActionOutput output={"one\ntwo\nthree\nfour\n"} />);
    expect(log.scrollTop).toBe(500);
  });
});

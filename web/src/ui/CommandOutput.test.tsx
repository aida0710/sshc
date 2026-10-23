import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CommandOutput } from "./CommandOutput";

// layOut gives the log the sizes jsdom does not compute: content taller than
// the box, so it can scroll.
function layOut(log: HTMLElement, scrollHeight: number) {
  Object.defineProperty(log, "clientHeight", { configurable: true, value: 100 });
  Object.defineProperty(log, "scrollHeight", { configurable: true, value: scrollHeight });
}

describe("CommandOutput", () => {
  it("shows nothing until the command writes something", () => {
    render(<CommandOutput output="" label="Output of sshc on that host" />);
    expect(screen.queryByRole("log")).not.toBeInTheDocument();
  });

  it("keeps the newest line in view until the person scrolls up, and again once they scroll back down", () => {
    const { rerender } = render(<CommandOutput output="one\n" label="Output of sshc on that host" />);
    const log = screen.getByRole("log", { name: "Output of sshc on that host" });
    layOut(log, 300);
    rerender(<CommandOutput output={"one\ntwo\n"} label="Output of sshc on that host" />);
    expect(log.scrollTop).toBe(300);

    log.scrollTop = 50;
    fireEvent.scroll(log);
    layOut(log, 400);
    rerender(<CommandOutput output={"one\ntwo\nthree\n"} label="Output of sshc on that host" />);
    expect(log.scrollTop).toBe(50);

    log.scrollTop = 300;
    fireEvent.scroll(log);
    layOut(log, 500);
    rerender(<CommandOutput output={"one\ntwo\nthree\nfour\n"} label="Output of sshc on that host" />);
    expect(log.scrollTop).toBe(500);
  });
});

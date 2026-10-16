import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { noTerminalProgress, type TerminalProgress } from "./osc9Progress";
import { TerminalProgressBar } from "./TerminalProgressBar";

function shown(progress: TerminalProgress) {
  return render(<LanguageProvider><TerminalProgressBar progress={progress} /></LanguageProvider>);
}

describe("TerminalProgressBar", () => {
  it("進み具合の値と幅を出す", () => {
    shown({ state: "normal", percent: 40 });
    const bar = screen.getByRole("progressbar", { name: "Program progress" });
    expect(bar).toHaveAttribute("aria-valuenow", "40");
    expect(bar.firstElementChild).toHaveStyle({ width: "40%" });
  });

  it("エラーは名前で分かるようにする", () => {
    shown({ state: "error", percent: 80 });
    expect(screen.getByRole("progressbar", { name: "Program progress (error)" })).toBeInTheDocument();
  });

  it("終わりの分からない進み具合は値を持たない", () => {
    shown({ state: "indeterminate" });
    expect(screen.getByRole("progressbar")).not.toHaveAttribute("aria-valuenow");
  });

  it("進み具合が無ければ何も出さない", () => {
    shown(noTerminalProgress);
    expect(screen.queryByRole("progressbar")).toBeNull();
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CrashBoundary } from "./CrashBoundary";

function Broken(): never {
  throw new TypeError("undefined is not a function");
}

describe("CrashBoundary", () => {
  // **真っ白な画面は、最悪の失敗の形である。** 何が起きたかを知る手段が
  // devtools しか無い機械では、報告できることが「白かった」以外に無くなる。
  it("names the failure instead of leaving an empty page", () => {
    // React は捕まえた例外を必ず console へ書く。テストの出力を汚すだけなので黙らせる。
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <CrashBoundary>
        <Broken />
      </CrashBoundary>,
    );
    expect(screen.getByText(/TypeError: undefined is not a function/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
    quiet.mockRestore();
  });

  it("stays out of the way when nothing throws", () => {
    render(
      <CrashBoundary>
        <p>the application</p>
      </CrashBoundary>,
    );
    expect(screen.getByText("the application")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reload" })).toBeNull();
  });
});

import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { storageKey } from "../i18n/locale";
import { CrashBoundary } from "./CrashBoundary";

function Broken(): never {
  throw new TypeError("undefined is not a function");
}

describe("CrashBoundary", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("names the failure instead of leaving an empty page", () => {
    window.localStorage.setItem(storageKey, "en");
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <CrashBoundary>
        <Broken />
      </CrashBoundary>,
    );
    expect(screen.getByRole("heading", { name: "sshc stopped rendering" })).toBeInTheDocument();
    expect(screen.getByText(/They contain only the failure details/)).toBeInTheDocument();
    expect(screen.getByText(/TypeError: undefined is not a function/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
    quiet.mockRestore();
  });

  it("uses the saved Japanese locale for the fallback", () => {
    window.localStorage.setItem(storageKey, "ja");
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <CrashBoundary>
        <Broken />
      </CrashBoundary>,
    );
    expect(screen.getByRole("heading", { name: "sshc の画面を表示できませんでした" })).toBeInTheDocument();
    expect(screen.getByText(/下の内容にはエラー情報だけが含まれています/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "再読み込み" })).toBeInTheDocument();
    expect(screen.getByText(/TypeError: undefined is not a function/)).toBeInTheDocument();
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

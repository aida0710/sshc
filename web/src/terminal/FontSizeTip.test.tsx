import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { FontSizeTip } from "./FontSizeTip";
import { fontSizeTipVisibleMs, useFontSizeTip, type FontSizeTip as FontSizeTipState } from "./useFontSizeTip";

let tip: FontSizeTipState;

function Tip({ size, onReset = () => {} }: { size: number; onReset?: () => void }) {
  tip = useFontSizeTip();
  return <FontSizeTip tip={tip} size={size} settingsSize={13} onReset={onReset} />;
}

function renderTip(size: number, onReset?: () => void) {
  return render(<LanguageProvider initial="en"><Tip size={size} {...(onReset === undefined ? {} : { onReset })} /></LanguageProvider>);
}

describe("FontSizeTip", () => {
  beforeEach(() => { vi.useFakeTimers(); });
  afterEach(() => { vi.useRealTimers(); });

  it("shows nothing until the size is changed, then the size and a reset to the size in Settings", () => {
    const reset = vi.fn();
    renderTip(16, reset);
    expect(screen.queryByRole("status")).toBeNull();

    act(() => tip.show());
    expect(screen.getByRole("status")).toHaveTextContent("16px");
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(reset).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Reset" })).toHaveAttribute("title", "Back to the size in Settings (13px)");
  });

  it("offers no reset at the size in Settings", () => {
    renderTip(13);
    act(() => tip.show());
    expect(screen.getByRole("status")).toHaveTextContent("13px");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("goes away a while after the last change, but not while the pointer is on it", () => {
    renderTip(16);
    act(() => tip.show());
    act(() => { vi.advanceTimersByTime(fontSizeTipVisibleMs - 1); });
    act(() => tip.show());
    act(() => { vi.advanceTimersByTime(fontSizeTipVisibleMs - 1); });
    expect(screen.getByRole("status")).toBeInTheDocument();

    fireEvent.pointerEnter(screen.getByRole("status"));
    act(() => tip.show());
    act(() => { vi.advanceTimersByTime(fontSizeTipVisibleMs * 2); });
    expect(screen.getByRole("status")).toBeInTheDocument();

    fireEvent.pointerLeave(screen.getByRole("status"));
    act(() => { vi.advanceTimersByTime(fontSizeTipVisibleMs); });
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("keeps the focus where it was when the reset is pressed", () => {
    renderTip(16);
    act(() => tip.show());
    expect(fireEvent.mouseDown(screen.getByRole("button", { name: "Reset" }))).toBe(false);
  });
});

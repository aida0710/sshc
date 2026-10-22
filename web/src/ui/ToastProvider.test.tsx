import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, toastVisibleMs } from "./ToastProvider";
import { useToast } from "./useToast";

function Trigger({ message, tone }: { message: string; tone?: "success" }) {
  const showToast = useToast();
  return <button type="button" onClick={() => showToast(message, tone === undefined ? undefined : { tone })}>{message}</button>;
}

describe("ToastProvider", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("shows a notice in the live region and takes it away after it has been read", () => {
    render(<ToastProvider><Trigger message="Saved." /></ToastProvider>);
    act(() => screen.getByRole("button", { name: "Saved." }).click());
    expect(screen.getByRole("status")).toHaveTextContent("Saved.");

    act(() => { vi.advanceTimersByTime(toastVisibleMs); });
    expect(screen.getByRole("status")).toHaveTextContent("");
  });

  it("replaces the previous notice and gives the new one its full time", () => {
    render(<ToastProvider><Trigger message="First." /><Trigger message="Second." /></ToastProvider>);
    act(() => screen.getByRole("button", { name: "First." }).click());
    act(() => { vi.advanceTimersByTime(toastVisibleMs - 100); });
    act(() => screen.getByRole("button", { name: "Second." }).click());
    expect(screen.getByRole("status")).toHaveTextContent("Second.");
    expect(screen.getByRole("status")).not.toHaveTextContent("First.");

    act(() => { vi.advanceTimersByTime(toastVisibleMs - 100); });
    expect(screen.getByRole("status")).toHaveTextContent("Second.");
  });

  it("does nothing outside the provider, so a part can be drawn on its own", () => {
    render(<Trigger message="Alone." />);
    act(() => screen.getByRole("button", { name: "Alone." }).click());
    expect(screen.queryByRole("status")).toBeNull();
  });
});

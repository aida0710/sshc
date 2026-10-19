import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLongPress } from "./useLongPress";

function Pressable({ onLongPress, onClick }: { onLongPress: () => void; onClick: () => void }) {
  const longPress = useLongPress();
  return (
    <button
      type="button"
      onPointerDown={(event) => longPress.start(event, onLongPress)}
      onPointerUp={longPress.cancel}
      onClick={() => {
        if (!longPress.consumeClick()) onClick();
      }}
    >
      nas
    </button>
  );
}

describe("useLongPress", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("calls back after a held touch and swallows the click that follows", () => {
    const onLongPress = vi.fn();
    const onClick = vi.fn();
    render(<Pressable onLongPress={onLongPress} onClick={onClick} />);
    const button = screen.getByRole("button", { name: "nas" });
    fireEvent.pointerDown(button, { pointerType: "touch" });
    act(() => vi.advanceTimersByTime(500));
    fireEvent.pointerUp(button, { pointerType: "touch" });
    fireEvent.click(button);
    expect(onLongPress).toHaveBeenCalledOnce();
    expect(onClick).not.toHaveBeenCalled();
  });

  it("leaves a short tap as a click", () => {
    const onLongPress = vi.fn();
    const onClick = vi.fn();
    render(<Pressable onLongPress={onLongPress} onClick={onClick} />);
    const button = screen.getByRole("button", { name: "nas" });
    fireEvent.pointerDown(button, { pointerType: "touch" });
    act(() => vi.advanceTimersByTime(100));
    fireEvent.pointerUp(button, { pointerType: "touch" });
    fireEvent.click(button);
    act(() => vi.advanceTimersByTime(1000));
    expect(onLongPress).not.toHaveBeenCalled();
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("ignores a held mouse button", () => {
    const onLongPress = vi.fn();
    render(<Pressable onLongPress={onLongPress} onClick={vi.fn()} />);
    fireEvent.pointerDown(screen.getByRole("button", { name: "nas" }), { pointerType: "mouse" });
    act(() => vi.advanceTimersByTime(1000));
    expect(onLongPress).not.toHaveBeenCalled();
  });
});

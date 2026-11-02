import { render } from "@testing-library/react";
import { useLayoutEffect } from "react";
import { describe, expect, it, vi } from "vitest";
import { useShellLayers } from "./useShellLayers";

// PressesBackOnDrawing presses Back as soon as the page is drawn: a layout effect, which comes before the effects that
// follow a drawing.
function PressesBackOnDrawing({ drawing, onBack }: { drawing: number; onBack: (drawing: number) => void }) {
  useShellLayers({ mobileLayout: true, ready: true, section: "Connections", onBackWithoutLayer: () => onBack(drawing) });
  useLayoutEffect(() => {
    window.dispatchEvent(new Event("sshc-android-back", { cancelable: true }));
  }, [drawing]);
  return null;
}

describe("useShellLayers", () => {
  it("hands Back to the handler of the page as last drawn, even before the effects that follow the drawing", () => {
    const onBack = vi.fn();
    const { rerender } = render(<PressesBackOnDrawing drawing={1} onBack={onBack} />);
    rerender(<PressesBackOnDrawing drawing={2} onBack={onBack} />);
    expect(onBack).toHaveBeenLastCalledWith(2);
  });
});

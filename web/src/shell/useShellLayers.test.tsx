import { render } from "@testing-library/react";
import { useLayoutEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
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

  describe("Back with text selected", () => {
    const pressBack = () => {
      const back = new Event("sshc-android-back", { cancelable: true });
      window.dispatchEvent(back);
      return back;
    };
    function Shell({ onBack }: { onBack: () => void }) {
      useShellLayers({ mobileLayout: true, ready: true, section: "Terminal", onBackWithoutLayer: onBack });
      return null;
    }
    afterEach(() => {
      document.getSelection()?.removeAllRanges();
      document.body.replaceChildren();
    });

    it("lets go of the selection first, and leaves the page on the next press", () => {
      const onBack = vi.fn();
      render(<Shell onBack={onBack} />);
      const text = document.body.appendChild(document.createElement("pre"));
      text.textContent = "selected output";
      document.getSelection()?.selectAllChildren(text);

      expect(pressBack().defaultPrevented).toBe(true);
      expect(document.getSelection()?.isCollapsed).toBe(true);
      expect(onBack).not.toHaveBeenCalled();

      pressBack();
      expect(onBack).toHaveBeenCalledOnce();
    });

    it("lets go of a selection in the field being typed in, keeping its text", () => {
      const onBack = vi.fn();
      render(<Shell onBack={onBack} />);
      const field = document.body.appendChild(document.createElement("textarea"));
      field.value = "a long request";
      field.focus();
      field.setSelectionRange(2, 6);

      expect(pressBack().defaultPrevented).toBe(true);
      expect(field.selectionStart).toBe(6);
      expect(field.selectionEnd).toBe(6);
      expect(field.value).toBe("a long request");
      expect(onBack).not.toHaveBeenCalled();
    });

    it("leaves the selection alone while a layer is there to close first", () => {
      function WithNavigation() {
        const { setNavigationOpen } = useShellLayers({ mobileLayout: true, ready: true, section: "Terminal", onBackWithoutLayer: () => {} });
        useLayoutEffect(() => { setNavigationOpen(true); }, [setNavigationOpen]);
        return null;
      }
      render(<WithNavigation />);
      const text = document.body.appendChild(document.createElement("pre"));
      text.textContent = "selected output";
      document.getSelection()?.selectAllChildren(text);

      expect(pressBack().defaultPrevented).toBe(true);
      expect(document.getSelection()?.isCollapsed).toBe(false);
    });
  });
});

import { act, fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { describe, expect, it } from "vitest";
import { useScrollGoesOn, type ScrollEdge } from "./useScrollGoesOn";

function Area({ edge }: { edge: ScrollEdge }) {
  const area = useRef<HTMLDivElement>(null);
  const goesOn = useScrollGoesOn(area, edge);
  return <div ref={area} data-testid="area" data-goes-on={String(goesOn)} />;
}

// measure sets what the browser would measure; jsdom lays nothing out.
function measure(element: HTMLElement, sizes: Record<string, number>) {
  for (const [name, value] of Object.entries(sizes)) {
    Object.defineProperty(element, name, { configurable: true, value, writable: true });
  }
}

describe("useScrollGoesOn", () => {
  it("says a row goes on while part of it is past the right edge, and not once it is scrolled to the end", () => {
    render(<Area edge="right" />);
    const row = screen.getByTestId("area");
    measure(row, { scrollWidth: 480, clientWidth: 350, scrollLeft: 0 });
    act(() => { fireEvent.scroll(row); });
    expect(row.dataset.goesOn).toBe("true");

    measure(row, { scrollWidth: 480, clientWidth: 350, scrollLeft: 130 });
    act(() => { fireEvent.scroll(row); });
    expect(row.dataset.goesOn).toBe("false");
  });

  it("says a form goes on while part of it is below the bottom edge, and not once it is scrolled to the end", () => {
    render(<Area edge="bottom" />);
    const form = screen.getByTestId("area");
    measure(form, { scrollHeight: 900, clientHeight: 600, scrollTop: 0 });
    act(() => { fireEvent.scroll(form); });
    expect(form.dataset.goesOn).toBe("true");

    measure(form, { scrollHeight: 900, clientHeight: 600, scrollTop: 300 });
    act(() => { fireEvent.scroll(form); });
    expect(form.dataset.goesOn).toBe("false");
  });
});

import { act, fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { describe, expect, it } from "vitest";
import { useMoreToTheRight } from "./useMoreToTheRight";

function Row() {
  const row = useRef<HTMLDivElement>(null);
  const more = useMoreToTheRight(row);
  return <div ref={row} data-testid="row" data-more={String(more)} />;
}

// widths sets what the browser would measure; jsdom lays nothing out.
function widths(element: HTMLElement, { scroll, client, left }: { scroll: number; client: number; left: number }) {
  Object.defineProperty(element, "scrollWidth", { configurable: true, value: scroll });
  Object.defineProperty(element, "clientWidth", { configurable: true, value: client });
  Object.defineProperty(element, "scrollLeft", { configurable: true, value: left, writable: true });
}

describe("useMoreToTheRight", () => {
  it("says the row goes on while part of it is past the right edge, and not once it is scrolled to the end", () => {
    render(<Row />);
    const row = screen.getByTestId("row");
    widths(row, { scroll: 480, client: 350, left: 0 });
    act(() => { fireEvent.scroll(row); });
    expect(row.dataset.more).toBe("true");

    widths(row, { scroll: 480, client: 350, left: 130 });
    act(() => { fireEvent.scroll(row); });
    expect(row.dataset.more).toBe("false");
  });
});

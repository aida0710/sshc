import { render, screen } from "@testing-library/react";
import { useRef } from "react";
import { describe, expect, it } from "vitest";
import { useFocusBackAfterInert } from "./useFocusBackAfterInert";

function Shell({ inert }: { inert: boolean }) {
  const container = useRef<HTMLDivElement>(null);
  useFocusBackAfterInert(container, inert);
  return (
    <>
      <div ref={container} inert={inert}><textarea aria-label="Terminal input" /></div>
      <button type="button">Outside</button>
    </>
  );
}

describe("useFocusBackAfterInert", () => {
  it("gives the focus back to the field that had it once the container is no longer inert", () => {
    const view = render(<Shell inert={false} />);
    const input = screen.getByLabelText("Terminal input");
    input.focus();

    view.rerender(<Shell inert />);
    // A browser moves the focus off an element that becomes inert; jsdom does not, so the test does.
    input.blur();
    expect(document.body).toHaveFocus();
    view.rerender(<Shell inert={false} />);

    expect(input).toHaveFocus();
  });

  it("leaves the focus where the person put it while the container was inert", () => {
    const view = render(<Shell inert={false} />);
    screen.getByLabelText("Terminal input").focus();

    view.rerender(<Shell inert />);
    screen.getByRole("button", { name: "Outside" }).focus();
    view.rerender(<Shell inert={false} />);

    expect(screen.getByRole("button", { name: "Outside" })).toHaveFocus();
  });
});

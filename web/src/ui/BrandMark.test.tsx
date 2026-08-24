import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { BrandMark } from "./BrandMark";

describe("BrandMark", () => {
  it("uses the official sshc application mark without exposing decorative text", () => {
    const { container } = render(<BrandMark className="h-8 w-8" />);
    const mark = container.querySelector("[data-sshc-brand-mark]");

    expect(mark).toHaveAttribute("aria-hidden", "true");
    expect(mark).toHaveAttribute("viewBox", "0 0 512 512");
    expect(mark).toHaveClass("h-8", "w-8");
    expect(mark?.querySelector('rect[fill="var(--ui-brand-background)"]')).not.toBeNull();
    expect(mark?.querySelector('path[d="M136 166H244L288 210H376V346H268L224 302H136Z"]')).not.toBeNull();
    expect(mark).not.toHaveTextContent(">_");
  });

  it("gives each rendered mark its own gradient definition", () => {
    const { container } = render(
      <>
        <BrandMark />
        <BrandMark />
      </>,
    );
    const gradients = [...container.querySelectorAll("linearGradient")];

    expect(gradients).toHaveLength(2);
    expect(gradients[0]?.id).not.toBe(gradients[1]?.id);
  });
});

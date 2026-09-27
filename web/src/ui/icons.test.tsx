import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Icon, iconNames } from "./icons";

describe("icons", () => {
  it("draws every name with a lucide icon", () => {
    for (const name of iconNames) {
      const { container, unmount } = render(<Icon name={name} />);
      const svg = container.querySelector("svg");
      expect(svg, name).not.toBeNull();
      expect(svg?.getAttribute("class"), name).toContain("lucide");
      expect(svg?.getAttribute("data-icon")).toBe(name);
      unmount();
    }
  });

  it("hides itself from the accessibility tree", () => {
    const { container } = render(<Icon name="keys" />);
    expect(container.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
  });

  it("keeps the size given by the caller", () => {
    const { container } = render(<Icon name="sync" className="size-5" />);
    expect(container.querySelector("svg")?.getAttribute("class")).toContain("size-5");
  });

  // 設定を開くボタンは歯車で見せる。手で描いていた形は太陽に見えた。
  it("uses the lucide gear for settings", () => {
    const { container } = render(<Icon name="settings" />);
    expect(container.querySelector("svg")?.getAttribute("class")).toContain("lucide-settings");
  });
});

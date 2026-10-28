import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EditorSection } from "./EditorSection";

describe("EditorSection", () => {
  it("names its part of the editor by its heading and keeps the details beside it", () => {
    render(<EditorSection title="接続先" meta="connections/lab/10-gpu.conf"><input aria-label="ホスト名" /></EditorSection>);

    const section = screen.getByRole("region", { name: "接続先" });
    expect(section).toContainElement(screen.getByLabelText("ホスト名"));
    expect(section).toHaveTextContent("connections/lab/10-gpu.conf");
  });
});

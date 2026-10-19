import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { NoticeList } from "./NoticeList";

const duplicate = {
  code: "duplicate_alias", path: "config", line: 3, detail: "nas",
  related: { path: "conf.d/10-home.conf", line: 1 },
};

describe("NoticeList", () => {
  it("names the duplicated alias and both places that declare it", () => {
    render(<NoticeList notices={[duplicate]} />);
    expect(screen.getByText(
      "nas is declared in both conf.d/10-home.conf:1 and config:3. For most settings OpenSSH uses the values from conf.d/10-home.conf:1, which it reads first. Delete the one you do not use, or rename it.",
    )).toBeInTheDocument();
  });

  it("falls back to the place alone when the engine sent no second place", () => {
    render(<NoticeList notices={[{ code: "duplicate_alias", path: "config", line: 41 }]} />);
    expect(screen.getByText(/Another block declares the same alias.*\(config:41\)/)).toBeInTheDocument();
  });

  it("folds many warnings into one line that opens on request", async () => {
    const { container } = render(<NoticeList collapsible notices={[duplicate, { ...duplicate, line: 9 }]} />);
    const details = container.querySelector("details");
    expect(details).not.toBeNull();
    expect(details).not.toHaveAttribute("open");
    await userEvent.click(screen.getByText("Warnings (2)"));
    expect(details).toHaveAttribute("open");
  });
});

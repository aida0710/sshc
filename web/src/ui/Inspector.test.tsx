import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InspectorPane, InspectorToggle } from "./Inspector";

describe("InspectorToggle", () => {
  it("says whether the pane is open", () => {
    render(<InspectorToggle label="Display and classification" open={false} attention={false} onToggle={vi.fn()} />);

    const button = screen.getByRole("button", { name: "Show Display and classification" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(button).toHaveAttribute("aria-controls", "inspector");
    expect(button).toHaveTextContent("Display and classification");
  });

  it("changes its name when open", () => {
    render(<InspectorToggle label="Display and classification" open attention={false} onToggle={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Hide Display and classification" })).toHaveAttribute("aria-expanded", "true");
  });

  // notice は既定で閉じているペインの中に住んでいるので、問題を抱えた
  // ホストは問題のないホストとまったく同じに見えてしまう。ドットこそが
  // ペインを開く価値のあるものにし、それはスクリーンリーダーにも届かなければならない。
  it("says so when what is inside needs attention", () => {
    render(<InspectorToggle label="Display and classification" open={false} attention onToggle={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Show Display and classification Needs attention" })).toBeInTheDocument();
  });

  // accessible name 全体への完全一致であり、それがこのテストを上の
  // テストの弱い版ではなく正反対のものにしている。
  it("does not say so otherwise", () => {
    render(<InspectorToggle label="Display and classification" open={false} attention={false} onToggle={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Show Display and classification" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Needs attention/ })).toBeNull();
  });

  it("reports a click", async () => {
    const onToggle = vi.fn();
    const user = userEvent.setup();
    render(<InspectorToggle label="Display and classification" open={false} attention={false} onToggle={onToggle} />);

    await user.click(screen.getByRole("button", { name: "Show Display and classification" }));

    expect(onToggle).toHaveBeenCalledOnce();
  });
});

describe("InspectorPane", () => {
  const single = {
    label: "Details",
    attention: false,
    panes: [{ key: "only", label: "Only", body: "nothing yet" }],
  };

  it("is a labelled complementary region the toggle can address", () => {
    render(<InspectorPane label="Details" content={single} />);

    const pane = screen.getByRole("complementary", { name: "Details" });
    expect(pane).toHaveAttribute("id", "inspector");
    expect(pane).toHaveTextContent("nothing yet");
  });

  // 1 面しか持たないセクションの見た目は、ペインが 2 面を持てるように
  // なる前と変わらない。切り替える先が無いのにセグメントを出すのは、
  // 押しても何も起きないコントロールを置くことである。
  it("draws no segmented control for a single face", () => {
    render(<InspectorPane label="Details" content={single} />);

    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });

  it("pins the header above both faces and switches only what is below it", async () => {
    const user = userEvent.setup();
    render(
      <InspectorPane
        label="Connection"
        content={{
          label: "Connection",
          attention: false,
          header: <p>ops@203.0.113.10:22</p>,
          panes: [
            { key: "consoles", label: "Consoles", body: <p>console list</p> },
            { key: "settings", label: "Settings", body: <p>display settings</p> },
          ],
        }}
      />,
    );

    // 先頭の面が既定で開いている。
    expect(screen.getByText("console list")).toBeInTheDocument();
    expect(screen.queryByText("display settings")).not.toBeInTheDocument();
    expect(screen.getByText("ops@203.0.113.10:22")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Settings" }));

    expect(screen.getByText("display settings")).toBeInTheDocument();
    expect(screen.queryByText("console list")).not.toBeInTheDocument();
    // 接続セクションは切り替わらない。どちらの面を見ていても、いま開いて
    // いるものが何かは見えていなければならない。
    expect(screen.getByText("ops@203.0.113.10:22")).toBeInTheDocument();
  });

  // 中身を持たないセクションはトグルすら出さない。ペインが 2 面を持てるように
  // なってもその規則は変わらない。
  it("renders nothing when a section has no pane", () => {
    const { container } = render(<InspectorPane label="Details" content={null} />);

    expect(container).toBeEmptyDOMElement();
  });
});

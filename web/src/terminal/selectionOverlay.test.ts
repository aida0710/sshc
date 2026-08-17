import { describe, expect, it, vi } from "vitest";
import { attachSelectionOverlay, overlayClass, selectionHeldIn } from "./selectionOverlay";

function harness(lines: string[]) {
  const container = document.createElement("div");
  document.body.appendChild(container);

  // xterm が建てる形だけを真似る。中身は要らない——見たいのは、板がこの
  // 部分木の**外**にぶら下がることである。
  const element = document.createElement("div");
  element.className = "xterm";
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  const rows = document.createElement("div");
  rows.className = "xterm-rows";
  screen.appendChild(rows);
  element.appendChild(screen);
  container.appendChild(element);

  let repaint = () => {};
  const view = {
    element,
    rows: lines.length,
    cols: 40,
    buffer: {
      active: {
        viewportY: 0,
        length: lines.length,
        getLine: (index: number) => {
          const line = lines[index];
          if (line === undefined) return undefined;
          return { translateToString: (trim?: boolean) => (trim ? line.trimEnd() : line) };
        },
      },
    },
    focus: vi.fn(),
    blur: vi.fn(),
    onRender: (handler: () => void) => {
      repaint = handler;
      return { dispose: vi.fn() };
    },
  };

  const detach = attachSelectionOverlay(container, view);
  const overlay = container.querySelector(`.${overlayClass}`) as HTMLElement;
  return { container, overlay, view, screen, detach, repaint: () => repaint() };
}

describe("attachSelectionOverlay", () => {
  // **これがこの設計そのものである。** .xterm の中では長押しからの選択が
  // 始まらないので、板は必ずその外にぶら下がらなければならない。
  it("hangs the text outside the element that cannot be selected", () => {
    const { container, overlay, detach } = harness(["one", "two"]);
    expect(overlay).not.toBeNull();
    expect(overlay.closest(".xterm")).toBeNull();
    expect(overlay.parentElement).toBe(container);
    detach();
  });

  it("carries the visible rows", () => {
    const { overlay, detach } = harness(["first line", "second line"]);
    expect(overlay.textContent).toBe("first line\nsecond line");
    detach();
  });

  // **選んでいる最中に写し替えない。** textContent を差し替えれば選択は消え、
  // ハンドルもコピーの吹き出しも一緒に消える。
  it("freezes while a selection is held in it", () => {
    const lines = ["before"];
    const { overlay, detach, repaint } = harness(lines);
    const range = document.createRange();
    range.selectNodeContents(overlay);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(overlay)).toBe(true);

    // 裏で端末が動いたことにして描き直させる。掴んでいる間は写し替えない。
    lines[0] = "after";
    repaint();
    expect(overlay.textContent).toBe("before");

    // 手を離せば、次の描き直しで追いつく。
    selection?.removeAllRanges();
    repaint();
    expect(overlay.textContent).toBe("after");
    detach();
  });

  // **形が変わったら、掴んでいたものはもう合わない。** キーボードが閉じれば
  // 窓の高さが変わり、xterm は全部を描き直す。字だけ止めておくと、帯は動いた
  // 字の上に残る——選んだつもりの範囲と、見えている範囲が食い違う。
  it("lets go of the selection when the terminal is laid out again", () => {
    const lines = ["before"];
    const { overlay, detach, repaint, screen } = harness(lines);
    const range = document.createRange();
    range.selectNodeContents(overlay);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(overlay)).toBe(true);

    lines[0] = "after";
    screen.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 200, height: 100 }) as DOMRect;
    repaint();

    expect(selectionHeldIn(overlay)).toBe(false);
    expect(overlay.textContent).toBe("after");
    detach();
  });

  // **これが無いと、叩いてもキーボードが出ない。** Chromium は touchend の
  // あとに mousedown を投げ、その既定動作は焦点を板の外——body——へ移す。
  // touchend で当てたばかりの textarea が、そこで外れる。
  it("swallows the mouse event Chromium sends after a finger", () => {
    const { overlay, detach } = harness(["one"]);
    const event = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
    overlay.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    detach();
  });

  // 触ったあと、打つつもりの指には焦点を渡す。渡さなければ、板が上に乗って
  // いる以上どこにも渡らない。
  it("hands the terminal the focus after a tap", () => {
    const { overlay, view, detach } = harness(["one"]);
    // jsdom は TouchEvent を持たない。読まれるのは touches[0].clientY だけである。
    const touch = new Event("touchstart", { bubbles: true });
    Object.defineProperty(touch, "touches", { value: [{ clientY: 10 }] });
    overlay.dispatchEvent(touch);
    overlay.dispatchEvent(new Event("touchend", { bubbles: true }));
    expect(view.focus).toHaveBeenCalled();
    detach();
  });

  it("takes the node away again", () => {
    const { container, detach } = harness(["one"]);
    detach();
    expect(container.querySelector(`.${overlayClass}`)).toBeNull();
  });
});

describe("selectionHeldIn", () => {
  it("is false for a collapsed caret", () => {
    const node = document.createElement("div");
    node.textContent = "text";
    document.body.appendChild(node);
    const range = document.createRange();
    range.setStart(node, 0);
    range.collapse(true);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(node)).toBe(false);
    selection?.removeAllRanges();
    node.remove();
  });

  // よそで選ばれているものは、こちらの都合ではない。
  it("is false when the selection lives elsewhere", () => {
    const mine = document.createElement("div");
    const theirs = document.createElement("div");
    theirs.textContent = "not mine";
    document.body.append(mine, theirs);
    const range = document.createRange();
    range.selectNodeContents(theirs);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(mine)).toBe(false);
    selection?.removeAllRanges();
    mine.remove();
    theirs.remove();
  });
});

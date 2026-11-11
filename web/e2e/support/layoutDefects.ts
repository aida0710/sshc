import type { Page } from "@playwright/test";

// What a person sees as a broken screen, listed by the page itself. A broken screen does not fail any other test: the
// button still has its name and still works, it is only cut off, or under the bar, or away from what it belongs to.
export type LayoutDefectKind =
  // Reaches past the edge of the window, with nothing to scroll sideways to bring it back.
  | "off-screen"
  // A box that scrolls sideways although it holds neither code nor a row of tabs.
  | "sideways-scroll"
  // Text cut off by a box that hides its overflow, with no title to read the rest in.
  | "clipped-text"
  // Scrolling to a control does not bring it out from under another element.
  | "covered-control"
  // Text running out of the card or field drawn around it.
  | "past-its-box"
  // A check box or radio button whose name is not right beside it on the same line.
  | "detached-checkbox"
  // Less than the 44px a finger needs, on a screen laid out as a phone.
  | "small-target"
  // An icon or a dot beside a line of text, higher or lower than the middle of the line.
  | "off-centre-mark"
  // Rows of one list built alike whose leading marks differ in size, or whose text starts at different places.
  | "uneven-rows"
  // The outline that marks the control in focus is cut away on one side by a box that hides its overflow.
  | "cut-focus-outline";

export type LayoutDefect = {
  kind: LayoutDefectKind;
  // What the person would call it: its accessible name or text.
  name: string;
  // The field row, dialog or region it is in.
  where: string;
  detail: string;
};

type ListingOptions = {
  // CSS selectors of what this listing does not judge. A code editor, for one, positions text and hidden helpers of
  // its own, and its overflow is its scrolling.
  ignored: readonly string[];
};

// listLayoutDefects runs inside the page, so everything it uses is declared in it: page.evaluate sends the function's
// text and nothing it closes over.
function listLayoutDefects({ ignored }: ListingOptions): LayoutDefect[] {
  // How far past an edge a box may lie before it counts. Boxes have fractional sizes, and a line's box is taller than
  // its letters, so a line fitted tightly into a row looks a pixel or two too tall.
  const sidewaysTolerancePx = 1;
  const heightTolerancePx = 3;
  // How far a check box's name may be from the box: the gap the shared CheckboxField leaves, with room to spare.
  const nameGapPx = 24;
  // The finger-sized heights: what the product promises on a phone, and for the packed lists marked data-touch-dense.
  const phoneTargetPx = 44;
  const denseTargetPx = 36;

  const defects: LayoutDefect[] = [];
  const windowWidth = document.documentElement.clientWidth;
  const windowHeight = window.innerHeight;
  const laidOutAsPhone = matchMedia("(max-width: 767px), (pointer: coarse) and (max-width: 1023px)").matches;

  // A modal dialog, an open menu or a details pane drawn over the page covers it on purpose, so only what is in it is
  // judged. A pane beside the page is not fixed in place, and does not count.
  const topLayers = [...document.querySelectorAll("[role='dialog'],[role='alertdialog'],[role='menu'],aside")].filter(
    (layer) => layer.getBoundingClientRect().width > 0 && (layer.tagName !== "ASIDE" || getComputedStyle(layer).position === "fixed"),
  );
  const topLayer = topLayers[topLayers.length - 1];

  const isJudged = (element: Element) =>
    !ignored.some((selector) => element.closest(selector) !== null) && (topLayer === undefined || topLayer.contains(element));

  const isShown = (element: Element) => {
    const box = element.getBoundingClientRect();
    // A visually hidden element is a 1px box kept for screen readers.
    if (box.width < 2 || box.height < 2) return false;
    if (element.closest("[aria-hidden='true'],[inert]") !== null) return false;
    // This also answers for what a closed <details> holds, which keeps its size while it is not drawn.
    return element.checkVisibility({ opacityProperty: true, visibilityProperty: true, contentVisibilityAuto: true });
  };

  const nameOf = (element: Element) =>
    (element.getAttribute("aria-label") ?? element.getAttribute("title") ?? element.getAttribute("placeholder") ?? element.textContent ?? "")
      .replace(/\s+/g, " ")
      .trim()
      .slice(0, 60) || element.tagName.toLowerCase();

  const whereOf = (element: Element) => {
    const holder = element.closest("[data-field-row],[role='dialog'],[role='region'],section[aria-label],nav[aria-label],main");
    if (holder === null) return "page";
    const label = holder.getAttribute("aria-label");
    if (label !== null) return `${holder.tagName.toLowerCase()} "${label}"`;
    return holder.hasAttribute("data-field-row") ? `row "${(holder.textContent ?? "").trim().slice(0, 30)}"` : holder.tagName.toLowerCase();
  };

  const note = (kind: LayoutDefectKind, element: Element, detail: string) => {
    defects.push({ kind, name: nameOf(element), where: whereOf(element), detail });
  };

  const ownTextNodes = (element: Element) =>
    [...element.childNodes].filter((node) => node.nodeType === Node.TEXT_NODE && (node.textContent ?? "").trim() !== "");

  const boxOfNodes = (nodes: readonly Node[]) => {
    const range = document.createRange();
    range.setStartBefore(nodes[0]!);
    range.setEndAfter(nodes[nodes.length - 1]!);
    return range.getBoundingClientRect();
  };

  const scrollsSideways = (style: CSSStyleDeclaration) => style.overflowX === "auto" || style.overflowX === "scroll";
  const hidesSideways = (style: CSSStyleDeclaration) => style.overflowX === "hidden" || style.overflowX === "clip";
  const hidesDown = (style: CSSStyleDeclaration) => style.overflowY === "hidden" || style.overflowY === "clip";

  const textElements = [...document.querySelectorAll("body *")].filter(
    (element) => isJudged(element) && ownTextNodes(element).length > 0 && isShown(element),
  );

  // ---- Reaching past the window, or scrolling sideways ----

  if (document.documentElement.scrollWidth > windowWidth + sidewaysTolerancePx) {
    defects.push({
      kind: "off-screen",
      name: "the page",
      where: "page",
      detail: `the page is ${document.documentElement.scrollWidth}px wide in a ${windowWidth}px window`,
    });
  }
  for (const element of document.querySelectorAll("body *")) {
    if (!isJudged(element)) continue;
    const style = getComputedStyle(element);
    if (!scrollsSideways(style) || element.scrollWidth <= element.clientWidth + sidewaysTolerancePx || element.clientWidth === 0) continue;
    // Code keeps its lines. A strip that hides its scrollbar (a row of tabs or filters) scrolls by design; what grew
    // too wide by accident shows a scrollbar.
    if (element.closest("pre,code") !== null || style.scrollbarWidth === "none") continue;
    if (!isShown(element)) continue;
    note("sideways-scroll", element, `holds ${element.scrollWidth}px in ${element.clientWidth}px`);
  }

  const offScreenCandidates = [
    ...textElements,
    ...document.querySelectorAll("button,a[href],input:not([type='hidden']),select,textarea,summary,[role='button']"),
  ];
  for (const element of new Set(offScreenCandidates)) {
    if (!isJudged(element) || !isShown(element)) continue;
    const box = element.getBoundingClientRect();
    if (box.right <= windowWidth + sidewaysTolerancePx && box.left >= -sidewaysTolerancePx) continue;
    let contained = false;
    for (let at = element.parentElement; at !== null && at !== document.body; at = at.parentElement) {
      const style = getComputedStyle(at);
      // Inside a box that hides or scrolls its overflow, it is that box's business: clipped-text reports what is cut.
      if (scrollsSideways(style) || hidesSideways(style)) {
        contained = true;
        break;
      }
    }
    if (!contained) note("off-screen", element, `spans ${Math.round(box.left)}px to ${Math.round(box.right)}px in a ${windowWidth}px window`);
  }

  // ---- Cut-off text ----

  for (const element of textElements) {
    const text = boxOfNodes(ownTextNodes(element));
    for (let at: Element | null = element; at !== null && at !== document.body; at = at.parentElement) {
      const style = getComputedStyle(at);
      // Text that overflows a box that scrolls can be scrolled to; the sideways-scroll check judges that box.
      if (scrollsSideways(style)) break;
      const box = at.getBoundingClientRect();
      const cutSideways = hidesSideways(style) && (text.right > box.right + sidewaysTolerancePx || text.left < box.left - sidewaysTolerancePx);
      const cutDown = hidesDown(style) && (text.bottom > box.bottom + heightTolerancePx || text.top < box.top - heightTolerancePx);
      if (!cutSideways && !cutDown) continue;
      if (element.closest("[title]") === null) {
        note("clipped-text", element, `${Math.round(text.width)}x${Math.round(text.height)}px of text in a ${Math.round(box.width)}x${Math.round(box.height)}px box`);
      }
      break;
    }
  }

  // ---- Text past the edge of the box drawn around it ----

  for (const element of textElements) {
    const text = boxOfNodes(ownTextNodes(element));
    for (let at: Element | null = element; at !== null && at !== document.body; at = at.parentElement) {
      const style = getComputedStyle(at);
      // A box that hides or scrolls its overflow has been judged above.
      if (scrollsSideways(style) || hidesSideways(style)) break;
      const drawn = parseFloat(style.borderRightWidth) > 0 || style.backgroundColor !== "rgba(0, 0, 0, 0)";
      if (!drawn) continue;
      const box = at.getBoundingClientRect();
      if (text.right > box.right + sidewaysTolerancePx) note("past-its-box", element, `text ends at ${Math.round(text.right)}px, its box at ${Math.round(box.right)}px`);
      break;
    }
  }

  // ---- A hint cut off inside its field ----

  // An input's own text scrolls with the caret, but an empty field's hint and a number have nowhere to scroll to.
  const measuring = document.createElement("canvas").getContext("2d")!;
  for (const input of document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("input,textarea")) {
    if (!isJudged(input) || !isShown(input)) continue;
    const shownText = input.value === "" ? input.placeholder : input.type === "number" ? input.value : "";
    if (shownText === "" || input.tagName === "TEXTAREA") continue;
    const style = getComputedStyle(input);
    measuring.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
    const room = input.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
    const needed = measuring.measureText(shownText).width;
    if (needed > room + sidewaysTolerancePx) note("clipped-text", input, `"${shownText.slice(0, 30)}" needs ${Math.round(needed)}px in ${Math.round(room)}px`);
  }

  // ---- A check box away from its name ----

  for (const box of document.querySelectorAll<HTMLInputElement>("input[type='checkbox'],input[type='radio']")) {
    if (!isJudged(box) || !isShown(box)) continue;
    const labelledBy = box.getAttribute("aria-labelledby");
    const labelElement = labelledBy === null ? box.labels?.[0] ?? null : document.getElementById(labelledBy.split(/\s+/)[0]!);
    if (labelElement === null || labelElement === undefined) continue;
    const walker = document.createTreeWalker(labelElement, NodeFilter.SHOW_TEXT);
    let firstText: Node | null = walker.nextNode();
    while (firstText !== null && (firstText.textContent ?? "").trim() === "") firstText = walker.nextNode();
    if (firstText === null) continue;
    const range = document.createRange();
    range.selectNodeContents(firstText);
    const lines = range.getClientRects();
    const firstLine = lines[0];
    if (firstLine === undefined) continue;
    const mark = box.getBoundingClientRect();
    const gap = firstLine.left >= mark.right ? firstLine.left - mark.right : mark.left - firstLine.right;
    const sameLine = Math.abs((firstLine.top + firstLine.bottom) / 2 - (mark.top + mark.bottom) / 2) <= Math.max(firstLine.height, mark.height) / 2;
    if (!sameLine || gap > nameGapPx) {
      note("detached-checkbox", box, `its name "${(firstText.textContent ?? "").trim().slice(0, 30)}" is ${Math.round(gap)}px across and ${sameLine ? "on the same line" : "on another line"}`);
    }
  }

  // ---- Marks beside text: level with their line, and alike down a list ----

  // A mark is a small picture that stands beside a line of text: an icon, a status dot. It is hidden from screen
  // readers more often than not, so it is not judged by isShown.
  const smallestMarkPx = 4;
  const largestMarkPx = 24;
  // How far a mark's middle may be from the middle of its line of text. A mark is measured against the line rather than
  // the letters, because where the letters sit in their line is the platform font's: within 1px of the line's middle on
  // Android and Windows, and a pixel higher on Linux at twice the pixel density. What this finds is a mark put off its
  // line by the page itself, such as a margin on a circle as tall as the line.
  const markCentreTolerancePx = 1;
  // How far the rows of one list may differ. Boxes have fractional sizes.
  const rowTolerancePx = 0.5;

  const isMark = (element: Element) => {
    if (!isJudged(element) || (element.parentElement?.closest("svg") ?? null) !== null) return false;
    const box = element.getBoundingClientRect();
    if (box.width < smallestMarkPx || box.height < smallestMarkPx || box.width > largestMarkPx || box.height > largestMarkPx) return false;
    if (!element.checkVisibility({ opacityProperty: true, visibilityProperty: true })) return false;
    if (element.tagName === "svg" || element.tagName === "IMG") return true;
    // A box drawn as a mark: a dot, a ring, or a circle holding one sign.
    if (element.children.length > 0 || (element.textContent ?? "").trim().length > 1) return false;
    const style = getComputedStyle(element);
    return style.backgroundColor !== "rgba(0, 0, 0, 0)" || style.backgroundImage !== "none" || parseFloat(style.borderTopWidth) > 0;
  };

  const firstTextIn = (element: Element) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
      const holder = node.parentElement;
      if ((node.textContent ?? "").trim() === "" || holder === null) continue;
      const box = holder.getBoundingClientRect();
      if (box.width < 2 || box.height < 2 || !holder.checkVisibility({ opacityProperty: true, visibilityProperty: true })) continue;
      return node as Text;
    }
    return null;
  };

  // The text a mark stands for: the first text after it in the row it is laid out in.
  const textBesideMark = (mark: Element) => {
    const row = mark.parentElement;
    if (row === null) return null;
    const style = getComputedStyle(row);
    if (!/flex|grid/.test(style.display) || style.flexDirection.startsWith("column")) return null;
    for (let sibling = mark.nextElementSibling; sibling !== null; sibling = sibling.nextElementSibling) {
      const text = firstTextIn(sibling);
      if (text !== null) return text;
    }
    return null;
  };

  // The box of a text's first line. The leading is shared out evenly above and below it, so its middle is the middle of
  // the line.
  const firstLineOf = (text: Text) => {
    const range = document.createRange();
    range.selectNodeContents(text);
    return range.getClientRects()[0] ?? null;
  };

  type MarkedRow = { mark: Element; box: DOMRect; textLeft: number; shape: string };
  const markedRowsByList = new Map<Element, MarkedRow[]>();

  for (const mark of document.querySelectorAll("body *")) {
    if (!isMark(mark)) continue;
    const text = textBesideMark(mark);
    if (text === null) continue;
    const line = firstLineOf(text);
    if (line === null) continue;
    const box = mark.getBoundingClientRect();
    const markMiddle = (box.top + box.bottom) / 2;
    const lineMiddle = (line.top + line.bottom) / 2;
    // A mark that stands beside the whole of a text of more lines than one is not lined up with its first line.
    if (markMiddle < line.top || markMiddle > line.bottom) continue;
    const offset = markMiddle - lineMiddle;
    if (Math.abs(offset) > markCentreTolerancePx) {
      // Marks are mostly nameless pictures, so each is told apart by the text it stands beside.
      const besideText = (text.textContent ?? "").trim().slice(0, 30);
      defects.push({ kind: "off-centre-mark", name: `${nameOf(mark)} beside "${besideText}"`, where: whereOf(mark), detail: `its middle is ${Math.abs(offset).toFixed(1)}px ${offset > 0 ? "below" : "above"} the middle of its line` });
    }

    const list = mark.closest("ul,ol,[role='list']");
    const row = mark.closest("li,[role='listitem']");
    if (list === null || row === null || row.closest("ul,ol,[role='list']") !== list) continue;
    const rows = markedRowsByList.get(list) ?? [];
    // Only the first mark of a row leads it.
    if (rows.some((marked) => row.contains(marked.mark))) continue;
    // Rows are compared with the rows built the same way: the same path from the row down to its mark.
    const path: string[] = [];
    for (let at: Element | null = mark; at !== null && at !== row; at = at.parentElement) path.unshift(at.tagName);
    rows.push({ mark, box, textLeft: line.left, shape: path.join(">") });
    markedRowsByList.set(list, rows);
  }

  for (const [list, rows] of markedRowsByList) {
    const byShape = new Map<string, MarkedRow[]>();
    for (const row of rows) byShape.set(row.shape, [...(byShape.get(row.shape) ?? []), row]);
    for (const alike of byShape.values()) {
      if (alike.length < 2) continue;
      const first = alike[0]!;
      const differences: string[] = [];
      const otherSize = alike.find((row) => Math.abs(row.box.width - first.box.width) > rowTolerancePx || Math.abs(row.box.height - first.box.height) > rowTolerancePx);
      if (otherSize !== undefined) {
        differences.push(`leading marks of ${first.box.width.toFixed(1)}x${first.box.height.toFixed(1)}px and ${otherSize.box.width.toFixed(1)}x${otherSize.box.height.toFixed(1)}px`);
      }
      const otherStart = alike.find((row) => Math.abs(row.textLeft - first.textLeft) > rowTolerancePx);
      if (otherStart !== undefined) {
        differences.push(`text starting ${Math.abs(otherStart.textLeft - first.textLeft).toFixed(1)}px apart after the marks`);
      }
      if (differences.length > 0) note("uneven-rows", list, differences.join("; "));
    }
  }

  // ---- Controls: can they be reached, and pressed with a finger? ----

  // Trying the controls scrolls whatever holds them, so where everything was scrolled to is put back afterwards.
  const scrolledBefore = [...document.querySelectorAll("body *")].filter((element) => element.scrollTop !== 0 || element.scrollLeft !== 0);
  const scrollPositions = scrolledBefore.map((element) => [element, element.scrollTop, element.scrollLeft] as const);

  const controls = [
    ...document.querySelectorAll<HTMLElement>(
      "button,a[href],input:not([type='hidden']),select,textarea,summary,[role='button'],[role='tab'],[role='menuitem'],[role='switch'],[role='checkbox'],[role='radio']",
    ),
  ].filter(
    (element) =>
      isJudged(element) &&
      isShown(element) &&
      !(element as HTMLButtonElement).disabled &&
      getComputedStyle(element).pointerEvents !== "none",
  );

  const landsOn = (control: Element, x: number, y: number) => {
    const hit = document.elementFromPoint(x, y);
    if (hit === null) return false;
    if (hit === control || control.contains(hit)) return true;
    // A press on the label around a check box goes to the box.
    return hit.closest("label") !== null && hit.closest("label") === control.closest("label");
  };

  // How many rows of pixels a finger at this x lands on the control, ::after and all.
  const pressHeight = (control: Element, x: number, middle: number) => {
    let top = middle;
    while (top > 0 && landsOn(control, x, top - 1)) top -= 1;
    let bottom = middle;
    while (bottom < windowHeight && landsOn(control, x, bottom + 1)) bottom += 1;
    return Math.round(bottom - top) + 1;
  };

  // The outline of a control in focus is 2px wide and 2px away from it (index.css). A control marked
  // sshc-focus-inset draws it inside itself, where nothing cuts it.
  const outlineReachPx = 4;
  // sideWhereFocusOutlineIsCut names the side on which a box that hides its overflow leaves no room for the whole
  // outline, with the box. A control used by touch alone shows no outline, so a phone's layout is not judged.
  const sideWhereFocusOutlineIsCut = (control: Element, box: DOMRect): { toString(): string; by: string } | null => {
    if (laidOutAsPhone || control.classList.contains("sshc-focus-inset")) return null;
    // A field that is hidden and drawn by the element after it (a switch, a swatch) shows the outline on that one.
    if (getComputedStyle(control).opacity === "0") return null;
    for (let holder = control.parentElement; holder !== null; holder = holder.parentElement) {
      const style = getComputedStyle(holder);
      const cutsSideways = style.overflowX !== "visible";
      const cutsDown = style.overflowY !== "visible";
      if (!cutsSideways && !cutsDown) continue;
      const outer = holder.getBoundingClientRect();
      // What the holder shows is inside its border.
      const shown = { left: outer.left + holder.clientLeft, top: outer.top + holder.clientTop, right: outer.left + holder.clientLeft + holder.clientWidth, bottom: outer.top + holder.clientTop + holder.clientHeight };
      const sides: Array<[string, boolean]> = [
        ["left", cutsSideways && holder.scrollLeft === 0 && box.left - outlineReachPx < shown.left - sidewaysTolerancePx],
        ["right", cutsSideways && holder.scrollLeft + holder.clientWidth >= holder.scrollWidth - 1 && box.right + outlineReachPx > shown.right + sidewaysTolerancePx],
        ["top", cutsDown && holder.scrollTop === 0 && box.top - outlineReachPx < shown.top - sidewaysTolerancePx],
        ["bottom", cutsDown && holder.scrollTop + holder.clientHeight >= holder.scrollHeight - 1 && box.bottom + outlineReachPx > shown.bottom + sidewaysTolerancePx],
      ];
      const cut = sides.find(([, isCut]) => isCut);
      if (cut !== undefined) {
        const by = `${holder.tagName.toLowerCase()}${holder.getAttribute("aria-label") === null ? "" : ` "${holder.getAttribute("aria-label")}"`}`;
        return { toString: () => cut[0], by };
      }
    }
    return null;
  };

  for (const control of controls) {
    control.scrollIntoView({ block: "center", inline: "nearest", behavior: "instant" });
    const box = control.getBoundingClientRect();
    const x = box.left + box.width / 2;
    const y = box.top + box.height / 2;
    if (x < 0 || x > windowWidth || y < 0 || y > windowHeight) {
      note("covered-control", control, `scrolling to it leaves its middle at ${Math.round(x)},${Math.round(y)}, outside the window`);
      continue;
    }
    if (!landsOn(control, x, y)) {
      const hit = document.elementFromPoint(x, y);
      note("covered-control", control, `a press at its middle lands on ${hit === null ? "nothing" : `<${hit.tagName.toLowerCase()}> "${nameOf(hit)}"`}`);
      continue;
    }
    const cutSide = sideWhereFocusOutlineIsCut(control, box);
    if (cutSide !== null) note("cut-focus-outline", control, `the outline around it is cut away on the ${cutSide} by <${cutSide.by}>`);
    if (!laidOutAsPhone || control.hasAttribute("data-touch-compact")) continue;
    // A link in a line of text is the height of the line. It is pressed as a part of what is being read.
    if (control.tagName === "A" && getComputedStyle(control).display === "inline") continue;
    const needed = control.hasAttribute("data-touch-dense") ? denseTargetPx : phoneTargetPx;
    const height = pressHeight(control, x, y);
    if (height + sidewaysTolerancePx < needed) note("small-target", control, `${Math.round(box.width)}px wide, ${height}px tall to press`);
  }

  for (const element of document.querySelectorAll("body *")) {
    if (element.scrollTop !== 0 || element.scrollLeft !== 0) element.scrollTo({ top: 0, left: 0, behavior: "instant" });
  }
  for (const [element, top, left] of scrollPositions) element.scrollTo({ top, left, behavior: "instant" });

  const seen = new Set<string>();
  return defects.filter((defect) => {
    const key = `${defect.kind} ${defect.name} ${defect.where}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

// The code editor (Monaco) draws its own text and hides its own helpers.
const notJudged = [".monaco-editor"];

// collectLayoutDefects lists what is broken on the screen as it stands. It scrolls to each control to try it, then puts
// every scroll position back.
export async function collectLayoutDefects(page: Page): Promise<LayoutDefect[]> {
  return page.evaluate(listLayoutDefects, { ignored: notJudged });
}

export function describeLayoutDefect(defect: LayoutDefect): string {
  return `${defect.kind}: "${defect.name}" in ${defect.where}: ${defect.detail}`;
}

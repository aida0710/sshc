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
  | "small-target";

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

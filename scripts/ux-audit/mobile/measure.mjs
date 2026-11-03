// capture-screens.mjs（ブランチ chore/ux-audit-2026-11-03-tools）の measure の写し。押す場所・切れた文字・はみ出しを測る。
export function measure(touch) {
  const viewportWidth = innerWidth;
  const viewportHeight = innerHeight;
  const minimumTarget = touch ? 44 : 24;
  // 名前の無いボタン（読み上げで何も言わない）も数える
  const unnamed = [...document.querySelectorAll('button, [role=button], a[href]')].filter((element) => element.getBoundingClientRect().width > 0 && !(element.getAttribute('aria-label') || element.textContent.trim() || element.getAttribute('title') || element.getAttribute('aria-labelledby'))).length;
  const onScreen = (element) => {
    const rectangle = element.getBoundingClientRect();
    if (rectangle.width === 0 || rectangle.height === 0) return false;
    if (rectangle.bottom <= 0 || rectangle.right <= 0 || rectangle.top >= viewportHeight || rectangle.left >= viewportWidth) return false;
    const style = getComputedStyle(element);
    if (style.visibility === "hidden" || Number(style.opacity) === 0) return false;
    return element.closest("[aria-hidden='true'],[inert]") === null;
  };
  const name = (element) => (element.getAttribute("aria-label") || element.getAttribute("title") || element.textContent || element.getAttribute("placeholder") || element.tagName).replace(/\s+/g, " ").trim().slice(0, 48);
  const targets = [...document.querySelectorAll("button, a[href], input:not([type=hidden]), select, textarea, summary, [role=button], [role=tab], [role=menuitem], [role=option], [role=switch], [role=checkbox], [role=radio]")]
    .filter((element) => !element.closest(".xterm"))
    .filter(onScreen);
  const smallTargets = targets
    .map((element) => ({ element, rectangle: element.getBoundingClientRect() }))
    .filter(({ element, rectangle }) => Math.min(rectangle.width, rectangle.height) < minimumTarget && !(element instanceof HTMLInputElement && ["checkbox", "radio"].includes(element.type) && element.closest("label") !== null))
    .map(({ element, rectangle }) => ({ name: name(element), width: Math.round(rectangle.width), height: Math.round(rectangle.height) }));
  const truncated = [...document.querySelectorAll("body *")]
    .filter((element) => {
      const style = getComputedStyle(element);
      const clipping = style.textOverflow === "ellipsis" || style.webkitLineClamp !== "none";
      return clipping && (element.scrollWidth > element.clientWidth + 1 || element.scrollHeight > element.clientHeight + 1) && onScreen(element);
    })
    .map((element) => element.textContent.replace(/\s+/g, " ").trim().slice(0, 60))
    .filter((text) => text !== "");
  const tinyText = [...document.querySelectorAll("body *")]
    .filter((element) => [...element.childNodes].some((node) => node.nodeType === Node.TEXT_NODE && node.textContent.trim() !== ""))
    .filter((element) => !element.closest(".xterm") && parseFloat(getComputedStyle(element).fontSize) < 12 && onScreen(element))
    .map((element) => `${getComputedStyle(element).fontSize} ${element.textContent.replace(/\s+/g, " ").trim().slice(0, 30)}`);
  const main = document.querySelector("main");
  const mainRectangle = main?.getBoundingClientRect();
  const scrollers = [...document.querySelectorAll("body *")].filter((element) => {
    const style = getComputedStyle(element);
    return /(auto|scroll)/.test(style.overflowY) && element.scrollHeight > element.clientHeight + 4 && onScreen(element);
  }).length;
  return {
    viewport: { width: viewportWidth, height: viewportHeight },
    media: Object.fromEntries(["(pointer: coarse)", "(hover: hover)", "(any-hover: hover)", "(any-pointer: coarse)"].map((query) => [query, matchMedia(query).matches])),
    overflowX: document.documentElement.scrollWidth > viewportWidth + 1,
    targets: targets.length,
    smallTargets,
    truncated,
    tinyText,
    main: mainRectangle === undefined ? null : { x: Math.round(mainRectangle.x), width: Math.round(mainRectangle.width), height: Math.round(mainRectangle.height) },
    scrollers,
    unnamed,
  };
}


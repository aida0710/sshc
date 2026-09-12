import { useLayoutEffect } from "react";

// Keep both the app and portalled dialogs inside the visible browser area.
// Android already removes IME/system insets from its WebView; its viewport
// reports that remaining area, so no native inset is subtracted a second time.
export function useAppViewport() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    const viewport = window.visualViewport;
    let fullHeight = window.innerHeight;
    let lastWidth = window.innerWidth;
    let mounted = true;
    let focusFrame: number | undefined;
    function update() {
      if (Math.abs(window.innerWidth - lastWidth) > 80) {
        lastWidth = window.innerWidth;
        fullHeight = window.innerHeight;
      }
      const active = document.activeElement;
      const editing = active instanceof HTMLElement &&
        (active.matches("input:not([type=checkbox]):not([type=radio]), textarea") || active.isContentEditable);
      // WebView can shrink innerHeight itself for the IME. Retain the largest
      // extent in this orientation, including when focus moves between fields.
      fullHeight = Math.max(fullHeight, window.innerHeight);
      // Do not resize the terminal's grid while the user pinch-zooms the page.
      if (viewport && Math.abs(viewport.scale - 1) > 0.01) return;
      const height = viewport?.height ?? window.innerHeight;
      root.style.setProperty("--app-viewport-height", `${height}px`);
      root.style.setProperty("--app-viewport-top", `${viewport?.offsetTop ?? 0}px`);
      const keyboardOpen = editing && Math.max(fullHeight, window.innerHeight) - height > 120;
      root.dataset.keyboardOpen = String(keyboardOpen);
      if (focusFrame !== undefined) cancelAnimationFrame(focusFrame);
      if (keyboardOpen && active instanceof HTMLElement && !active.closest("[data-terminal-host], .monaco-editor")) {
        // A modal's footer can obscure a field after the WebView shrinks or
        // focus changes. Reveal it inside its scroll container after layout.
        focusFrame = requestAnimationFrame(() => {
          if (document.activeElement === active) active.scrollIntoView?.({ block: "nearest", inline: "nearest" });
        });
      }
    }
    const afterFocus = () => queueMicrotask(() => { if (mounted) update(); });
    update();
    window.addEventListener("resize", update);
    viewport?.addEventListener("resize", update);
    viewport?.addEventListener("scroll", update);
    document.addEventListener("focusin", update);
    document.addEventListener("focusout", afterFocus);
    return () => {
      mounted = false;
      if (focusFrame !== undefined) cancelAnimationFrame(focusFrame);
      window.removeEventListener("resize", update);
      viewport?.removeEventListener("resize", update);
      viewport?.removeEventListener("scroll", update);
      document.removeEventListener("focusin", update);
      document.removeEventListener("focusout", afterFocus);
      root.style.removeProperty("--app-viewport-height");
      root.style.removeProperty("--app-viewport-top");
      delete root.dataset.keyboardOpen;
    };
  }, []);
}

import { useEffect, useRef, type RefObject } from "react";

export type DismissReason = "outside" | "escape" | "android-back" | "superseded";

type Layer = {
  id: symbol;
  containers: () => readonly (HTMLElement | null)[];
  dismiss: (reason: DismissReason) => void;
  closeOnOutside: boolean;
  restoreFocus: () => HTMLElement | null;
  initialFocus: () => HTMLElement | null;
  trapFocus: boolean;
};

const layers: Layer[] = [];

function topLayer(): Layer | undefined {
  return layers[layers.length - 1];
}

function dismissOutside(event: PointerEvent) {
  const layer = topLayer();
  const target = event.target;
  if (layer === undefined || !layer.closeOnOutside || !(target instanceof Node)) return;
  if (layer.containers().some((container) => container?.contains(target) === true)) return;
  layer.dismiss("outside");
}

function dismissWithEscape(event: KeyboardEvent) {
  const layer = topLayer();
  if (layer === undefined) return;
  if (event.key === "Tab" && layer.trapFocus) {
    const focusable = layer.containers().flatMap((container) =>
      container === null ? [] : [...container.querySelectorAll<HTMLElement>(
        'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])',
      )].filter((element) => !element.hasAttribute("disabled") && element.getAttribute("aria-hidden") !== "true"),
    );
    const first = focusable[0] ?? layer.containers()[0] ?? null;
    const last = focusable[focusable.length - 1] ?? first;
    if (first === null || last === null) return;
    const active = document.activeElement;
    if (event.shiftKey ? active === first || !focusable.includes(active as HTMLElement) : active === last || !focusable.includes(active as HTMLElement)) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    }
    return;
  }
  if (event.key !== "Escape") return;
  event.preventDefault();
  event.stopImmediatePropagation();
  const returnTarget = layer.restoreFocus();
  layer.dismiss("escape");
  queueMicrotask(() => {
    if (returnTarget?.isConnected === true) returnTarget.focus();
  });
}

function dismissForAndroidBack(event: Event) {
  const layer = topLayer();
  if (layer === undefined) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  const returnTarget = layer.restoreFocus();
  layer.dismiss("android-back");
  queueMicrotask(() => {
    if (returnTarget?.isConnected === true) returnTarget.focus();
  });
}

function listen() {
  if (layers.length !== 1) return;
  document.addEventListener("pointerdown", dismissOutside, true);
  document.addEventListener("keydown", dismissWithEscape, true);
  window.addEventListener("sshc-android-back", dismissForAndroidBack, true);
}

function unlisten() {
  if (layers.length !== 0) return;
  document.removeEventListener("pointerdown", dismissOutside, true);
  document.removeEventListener("keydown", dismissWithEscape, true);
  window.removeEventListener("sshc-android-back", dismissForAndroidBack, true);
}

export function useDismissibleLayer({
  open,
  containerRefs,
  onDismiss,
  closeOnOutside = true,
  returnFocusRef,
  initialFocusRef,
  trapFocus = false,
}: {
  open: boolean;
  containerRefs: readonly RefObject<HTMLElement | null>[];
  onDismiss: (reason: DismissReason) => void;
  closeOnOutside?: boolean;
  returnFocusRef?: RefObject<HTMLElement | null>;
  initialFocusRef?: RefObject<HTMLElement | null>;
  trapFocus?: boolean;
}) {
  const id = useRef(Symbol("dismissible-layer"));
  const containers = useRef(containerRefs);
  const dismiss = useRef(onDismiss);
  const returnFocus = useRef(returnFocusRef);
  const initialFocus = useRef(initialFocusRef);
  containers.current = containerRefs;
  dismiss.current = onDismiss;
  returnFocus.current = returnFocusRef;
  initialFocus.current = initialFocusRef;

  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const layer: Layer = {
      id: id.current,
      containers: () => containers.current.map((ref) => ref.current),
      dismiss: (reason) => dismiss.current(reason),
      closeOnOutside,
      restoreFocus: () => returnFocus.current?.current ?? opener,
      initialFocus: () => initialFocus.current?.current ?? null,
      trapFocus,
    };
    topLayer()?.dismiss("superseded");
    layers.push(layer);
    listen();
    if (trapFocus) {
      const target = layer.initialFocus() ?? layer.containers().flatMap((container) =>
        container === null ? [] : [...container.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])',
        )],
      )[0] ?? layer.containers()[0] ?? null;
      target?.focus();
    }
    return () => {
      const index = layers.findIndex((candidate) => candidate.id === layer.id);
      if (index >= 0) layers.splice(index, 1);
      unlisten();
    };
  }, [closeOnOutside, open, trapFocus]);
}

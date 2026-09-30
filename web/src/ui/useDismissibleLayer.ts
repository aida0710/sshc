import { useEffect, useRef, type RefObject } from "react";

export type DismissReason = "outside" | "escape" | "android-back" | "superseded";

type Layer = {
  id: symbol;
  containers: () => readonly (HTMLElement | null)[];
  dismiss: (reason: DismissReason) => void;
  // false の間は、Escape・戻る操作・外側のクリックで閉じず、フォーカスも動かさない。
  dismissible: () => boolean;
  closeOnOutside: boolean;
  restoreFocus: () => HTMLElement | null;
  initialFocus: () => HTMLElement | null;
  trapFocus: boolean;
};

const layers: Layer[] = [];

const escapeOwnerAttribute = "data-escape-owner";

// エディタの検索欄や補完を閉じる、シェルへ ESC を送るなど、Escape を自分で使う領域に付ける。
// この中で押した Escape は先にその領域へ渡し、使われずに document まで届いたときだけ
// 最前面のレイヤーを閉じる。
export const escapeOwnerProps = { [escapeOwnerAttribute]: "" } as const;

function insideEscapeOwner(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(`[${escapeOwnerAttribute}]`) !== null;
}

function topLayer(): Layer | undefined {
  return layers[layers.length - 1];
}

function visible(element: HTMLElement): boolean {
  if (element.closest('[hidden], [inert], [aria-hidden="true"]')) return false;
  for (let node: HTMLElement | null = element; node; node = node.parentElement) {
    const style = getComputedStyle(node);
    if (style.display === "none" || style.visibility === "hidden") return false;
  }
  return true;
}

function focusableElements(container: HTMLElement): HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(
    'a[href], button, input, select, textarea, [tabindex]',
  )].filter((element) => !element.hasAttribute("disabled") && element.tabIndex >= 0 && visible(element));
}

function dismissOutside(event: PointerEvent) {
  const layer = topLayer();
  const target = event.target;
  if (layer === undefined || !layer.closeOnOutside || !layer.dismissible() || !(target instanceof Node)) return;
  if (layer.containers().some((container) => container?.contains(target) === true)) return;
  layer.dismiss("outside");
}

function dismissWithEscape(event: KeyboardEvent) {
  const layer = topLayer();
  if (layer === undefined) return;
  if (event.key === "Tab" && layer.trapFocus) {
    const focusable = layer.containers().flatMap((container) =>
      container === null ? [] : focusableElements(container),
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
  if (event.key !== "Escape" || insideEscapeOwner(event.target)) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  dismissAndRestoreFocus(layer, "escape");
}

// Monaco と xterm は使った Escape の伝播を止める。ここまで届いたものは使われなかった Escape。
function dismissWithUnusedEscape(event: KeyboardEvent) {
  const layer = topLayer();
  if (layer === undefined || event.key !== "Escape" || event.defaultPrevented || !insideEscapeOwner(event.target)) return;
  event.preventDefault();
  dismissAndRestoreFocus(layer, "escape");
}

function dismissAndRestoreFocus(layer: Layer, reason: DismissReason) {
  // 閉じないのにフォーカスだけ開いた元へ戻すと、開いたままのダイアログから離れる。
  if (!layer.dismissible()) return;
  const returnTarget = layer.restoreFocus();
  layer.dismiss(reason);
  queueMicrotask(() => {
    if (returnTarget?.isConnected === true) returnTarget.focus();
  });
}

// A late terminal connection or background widget must not take focus from a modal.
function keepModalFocus(event: FocusEvent) {
  const index = layers.reduce((found, candidate, position) => candidate.trapFocus ? position : found, -1);
  const layer = layers[index];
  if (layer === undefined || !(event.target instanceof Node)) return;
  if (layers.slice(index).some((candidate) => candidate.containers().some((container) => container?.contains(event.target as Node)))) return;
  (layer.initialFocus() ?? layer.containers()[0])?.focus();
}

function dismissForAndroidBack(event: Event) {
  const layer = topLayer();
  if (layer === undefined) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  dismissAndRestoreFocus(layer, "android-back");
}

function listen() {
  if (layers.length !== 1) return;
  document.addEventListener("pointerdown", dismissOutside, true);
  document.addEventListener("keydown", dismissWithEscape, true);
  document.addEventListener("keydown", dismissWithUnusedEscape);
  document.addEventListener("focusin", keepModalFocus, true);
  window.addEventListener("sshc-android-back", dismissForAndroidBack, true);
}

function unlisten() {
  if (layers.length !== 0) return;
  document.removeEventListener("pointerdown", dismissOutside, true);
  document.removeEventListener("keydown", dismissWithEscape, true);
  document.removeEventListener("keydown", dismissWithUnusedEscape);
  document.removeEventListener("focusin", keepModalFocus, true);
  window.removeEventListener("sshc-android-back", dismissForAndroidBack, true);
}

export function useDismissibleLayer({
  open,
  containerRefs,
  onDismiss,
  dismissible = true,
  closeOnOutside = true,
  returnFocusRef,
  initialFocusRef,
  trapFocus = false,
}: {
  open: boolean;
  containerRefs: readonly RefObject<HTMLElement | null>[];
  onDismiss: (reason: DismissReason) => void;
  dismissible?: boolean;
  closeOnOutside?: boolean;
  returnFocusRef?: RefObject<HTMLElement | null>;
  initialFocusRef?: RefObject<HTMLElement | null>;
  trapFocus?: boolean;
}) {
  const id = useRef(Symbol("dismissible-layer"));
  const containers = useRef(containerRefs);
  const dismiss = useRef(onDismiss);
  const canDismiss = useRef(dismissible);
  const returnFocus = useRef(returnFocusRef);
  const initialFocus = useRef(initialFocusRef);
  containers.current = containerRefs;
  dismiss.current = onDismiss;
  canDismiss.current = dismissible;
  returnFocus.current = returnFocusRef;
  initialFocus.current = initialFocusRef;

  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const layer: Layer = {
      id: id.current,
      containers: () => containers.current.map((ref) => ref.current),
      dismiss: (reason) => dismiss.current(reason),
      dismissible: () => canDismiss.current,
      closeOnOutside,
      restoreFocus: () => returnFocus.current?.current ?? opener,
      initialFocus: () => initialFocus.current?.current ?? null,
      trapFocus,
    };
    // React clears element refs before running effect cleanup. Keep the mounted
    // nodes so a close-button unmount can still tell that it owned focus.
    const mountedContainers = layer.containers();
    const previous = topLayer();
    // A menu opened inside a modal belongs to that modal. Keep its parent
    // mounted so Back/Escape can dismiss the menu before closing the sheet.
    const nested = previous?.trapFocus === true && opener !== null &&
      previous.containers().some((container) => container?.contains(opener));
    if (!nested) previous?.dismiss("superseded");
    layers.push(layer);
    listen();
    if (trapFocus) {
      const target = layer.initialFocus() ?? layer.containers().flatMap((container) =>
        container === null ? [] : focusableElements(container),
      )[0] ?? layer.containers()[0] ?? null;
      target?.focus();
    }
    return () => {
      const returnTarget = layer.restoreFocus();
      const index = layers.findIndex((candidate) => candidate.id === layer.id);
      if (index >= 0) layers.splice(index, 1);
      unlisten();
      // A close button or a successful submit unmounts the layer without going
      // through Escape. Restore only if focus was still owned by this layer;
      // an outside click or a replacement layer keeps its newly chosen focus.
      queueMicrotask(() => {
        const current = document.activeElement;
        const stillOwned = current instanceof Node && mountedContainers.some((container) => container?.contains(current) === true);
        if (returnTarget?.isConnected === true && (current === document.body || stillOwned)) returnTarget.focus();
      });
    };
  }, [closeOnOutside, open, trapFocus]);
}

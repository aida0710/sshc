import { useEffect, useRef, type RefObject } from "react";

function items(menu: HTMLElement): HTMLElement[] {
  return [...menu.querySelectorAll<HTMLElement>('[role="menuitem"], [role="menuitemcheckbox"], [role="menuitemradio"]')]
    .filter((item) => !item.hasAttribute("disabled") && item.getAttribute("aria-disabled") !== "true");
}

export function useMenuKeyboard({
  open,
  menuRef,
  onClose,
}: {
  open: boolean;
  menuRef: RefObject<HTMLElement | null>;
  onClose: () => void;
}) {
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    if (!open) return;
    const menu = menuRef.current;
    if (menu === null) return;
    const activeMenu = menu;
    items(activeMenu)[0]?.focus();

    function navigate(event: KeyboardEvent) {
      const available = items(activeMenu);
      if (available.length === 0) return;
      const current = available.indexOf(document.activeElement as HTMLElement);
      let next = -1;
      if (event.key === "ArrowDown") next = (current + 1) % available.length;
      else if (event.key === "ArrowUp") next = current <= 0 ? available.length - 1 : current - 1;
      else if (event.key === "Home") next = 0;
      else if (event.key === "End") next = available.length - 1;
      else if (event.key === "Tab") {
        queueMicrotask(() => close.current());
        return;
      } else return;
      event.preventDefault();
      available[next]?.focus();
    }

    activeMenu.addEventListener("keydown", navigate);
    return () => activeMenu.removeEventListener("keydown", navigate);
  }, [menuRef, open]);
}

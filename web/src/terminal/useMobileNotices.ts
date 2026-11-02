import { useEffect, useState, type RefObject } from "react";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";

// useMobileNotices is whether a phone shows its notices about the session, which it keeps behind a button in the top
// bar. They show only when the person asks, also for a new notice, and hide like a menu: with the button again, a tap
// outside them, Escape or Android's Back. They hide by themselves once there is nothing left to say.
export function useMobileNotices({ enabled, present, containerRefs }: {
  // enabled is whether the phone's way is used; a wider screen shows its notices at once.
  enabled: boolean;
  // present is whether there is any notice.
  present: boolean;
  // containerRefs are the notices shown and the button: a tap there does not hide them.
  containerRefs: readonly RefObject<HTMLElement | null>[];
}): { open: boolean; toggle: () => void; close: () => void } {
  const [requested, setRequested] = useState(false);
  const open = enabled && present && requested;
  useEffect(() => {
    if (!enabled || !present) setRequested(false);
  }, [enabled, present]);
  useDismissibleLayer({ open, containerRefs, onDismiss: () => setRequested(false) });
  return {
    open,
    toggle: () => setRequested((current) => !current),
    close: () => setRequested(false),
  };
}

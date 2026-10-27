import { useEffect, useRef, type ReactNode } from "react";

// InlinePanel holds what a row opens under itself (a form, a key, the codes around the current one), in
// place of a dialog over the page. It brings itself into view, since the row it belongs to may sit at the
// bottom of the window.
export function InlinePanel({ children, className = "" }: { children: ReactNode; className?: string }) {
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    panel.current?.scrollIntoView?.({ block: "nearest" });
  }, []);
  return <div ref={panel} className={className}>{children}</div>;
}

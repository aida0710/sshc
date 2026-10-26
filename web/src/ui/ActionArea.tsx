import type { ReactNode } from "react";

// ActionArea is the row under a form: what happened on the left, what to press on the right, with a
// line above it as in the design.
export function ActionArea({ children, status }: { children: ReactNode; status?: ReactNode }) {
  return (
    <div className="mt-6 flex min-h-12 flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
      <div className="min-w-0 flex-1">{status}</div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

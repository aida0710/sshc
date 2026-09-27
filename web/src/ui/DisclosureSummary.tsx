import type { ReactNode } from "react";
import { DisclosureChevron } from "./DisclosureChevron";

// DisclosureSummary は、<details> の見出しである。ブラウザが出す三角の印を消し、ほかの開閉と
// 同じ lucide の印（DisclosureChevron）を先頭に出す。
export function DisclosureSummary({ className = "", children }: { className?: string; children: ReactNode }) {
  return (
    <summary className={`flex cursor-pointer list-none items-center gap-1.5 [&::-webkit-details-marker]:hidden ${className}`}>
      <DisclosureChevron className="size-3.5" />
      {children}
    </summary>
  );
}

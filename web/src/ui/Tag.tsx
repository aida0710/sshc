import type { ReactNode } from "react";

export type TagTone = "neutral" | "ok" | "warn" | "accent";

// A tag is a short state or count beside a name, as in the design's .tag: 20px tall, fully rounded, 12px.
// neutral says a fact, ok that something works, warn that it needs a look, accent that it is the one chosen.
const tagBase = "inline-flex h-5 items-center gap-1 whitespace-nowrap rounded-full px-[7px] text-xs";

const tagTones: Record<TagTone, string> = {
  neutral: "bg-well text-ink-muted",
  ok: "bg-live/15 text-live",
  warn: "bg-notice text-notice-ink",
  accent: "bg-select-fill text-accent",
};

// tagClass is a tag's look, for an element that is already a span or a cell of its own.
export function tagClass(tone: TagTone): string {
  return `${tagBase} shrink-0 ${tagTones[tone]}`;
}

// 札はふつう縮まない。truncate の札は、利用者が付けた名前のような長さの決まらない文字を持つので、置き場所の
// 幅に合わせて縮み、入りきらない分を「…」にする。全文は title で読める。
export function Tag({ tone = "neutral", truncate = false, children }: { tone?: TagTone; truncate?: boolean; children: ReactNode }) {
  if (!truncate) return <span className={tagClass(tone)}>{children}</span>;
  return (
    <span className={`${tagBase} min-w-0 ${tagTones[tone]}`} title={typeof children === "string" ? children : undefined}>
      <span className="truncate">{children}</span>
    </span>
  );
}

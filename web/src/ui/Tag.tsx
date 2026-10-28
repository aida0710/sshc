import type { ReactNode } from "react";

export type TagTone = "neutral" | "ok" | "warn" | "accent";

// A tag is a short state or count beside a name, as in the design's .tag: 20px tall, fully rounded, 11.5px.
// neutral says a fact, ok that something works, warn that it needs a look, accent that it is the one chosen.
const tagBase = "inline-flex h-5 shrink-0 items-center gap-1 whitespace-nowrap rounded-full px-[7px] text-[11.5px]";

const tagTones: Record<TagTone, string> = {
  neutral: "bg-well text-ink-muted",
  ok: "bg-live/15 text-live",
  warn: "bg-notice text-notice-ink",
  accent: "bg-select-fill text-accent",
};

// tagClass is a tag's look, for an element that is already a span or a cell of its own.
export function tagClass(tone: TagTone): string {
  return `${tagBase} ${tagTones[tone]}`;
}

export function Tag({ tone = "neutral", children }: { tone?: TagTone; children: ReactNode }) {
  return <span className={tagClass(tone)}>{children}</span>;
}

import type { ReactNode } from "react";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

type SectionBarProps = {
  title: string;
  // 件数など、名前の横に小さく添えるもの。
  meta?: ReactNode;
  // 右端に置く、その画面の主な操作。
  actions?: ReactNode;
};

// SectionBar は、Home・Connections・SFTP の上に置く細い帯で、画面の名前・件数・主な操作を1行に並べる。
// どの画面でも同じ高さにするため、ボタンが無くても高さを保つ。
// スマホでは上のアプリの帯が画面の名前を出すので、名前は読み上げにだけ残し、何も見せるものが無ければ帯を出さない。
export function SectionBar({ title, meta = null, actions = null }: SectionBarProps) {
  const compact = useMediaQuery(mobileViewportQuery);
  const heading = <h1 className={compact ? "sr-only" : "truncate text-sm font-semibold tracking-tight text-ink"}>{title}</h1>;
  if (compact && meta === null && actions === null) return heading;
  return (
    <header data-section-bar className="flex min-h-14 shrink-0 items-center justify-between gap-3 border-b border-line bg-card px-3 py-2 md:px-4">
      <div className="flex min-w-0 items-baseline gap-2">
        {heading}
        {meta === null ? null : <p className="shrink-0 text-xs text-ink-muted">{meta}</p>}
      </div>
      {actions === null ? null : <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

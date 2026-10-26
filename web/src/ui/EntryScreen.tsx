import type { ReactNode } from "react";
import { BrandMark } from "./BrandMark";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

// EntryScreen is a page shown before the app opens: creating or unlocking the vault, a session that
// ended, an engine that cannot be reached. On a wide screen it is one wide card, as in the design, so it
// does not grow tall: sshc's mark, the heading and what is asked on a tinted left side, and the form on
// the right. A phone stacks the two. corner holds what belongs to the page rather than to the card
// (the theme and the language).
export function EntryScreen({
  heading,
  description,
  asideNote,
  corner,
  footer,
  children,
}: {
  heading: string;
  // description says what the screen asks for. A ReactNode, so a failure can be announced (role="alert").
  description: ReactNode;
  // asideNote sits at the bottom of the left side, such as the warning that a master password cannot be recovered.
  asideNote?: ReactNode;
  corner?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  const intro = (
    <>
      <BrandMark className="mb-[18px] size-10" />
      <h1 className="text-[22px] font-semibold leading-8 text-ink">{heading}</h1>
      <div className="mt-2 text-[13px] leading-5 text-ink-muted">{description}</div>
    </>
  );
  const footerArea = footer === undefined ? null : <div className="mt-5 border-t border-line pt-3.5">{footer}</div>;
  return (
    <main className={`relative flex min-h-screen bg-canvas text-ink ${compact ? "flex-col justify-center gap-2.5 px-3.5 py-4" : "items-center justify-center p-6"}`}>
      {corner === undefined ? null : (
        <div className={compact ? "flex justify-end gap-2" : "absolute right-4 top-4 flex gap-2"}>{corner}</div>
      )}
      {compact ? (
        <section className="rounded-2xl border border-line bg-sidebar px-5 pb-[18px] pt-6 shadow-pane">
          {intro}
          {asideNote === undefined ? null : <div className="mt-4">{asideNote}</div>}
          <div className="mt-4">{children}</div>
          {footerArea}
        </section>
      ) : (
        <section className="grid w-full max-w-[760px] grid-cols-[300px_minmax(0,1fr)] overflow-hidden rounded-2xl border border-line bg-sidebar shadow-pane">
          <div className="flex flex-col border-r border-line bg-[color-mix(in_srgb,var(--ui-accent)_9%,var(--ui-sidebar))] p-8">
            {intro}
            {asideNote === undefined ? null : <div className="mt-auto pt-6">{asideNote}</div>}
          </div>
          <div className="flex flex-col justify-center p-8">
            {children}
            {footerArea}
          </div>
        </section>
      )}
    </main>
  );
}

import { useId, type ReactNode } from "react";

// EditorSection is one part of a tab of the connection editor, set off from the part above by a line: a heading,
// details beside it on the right (meta, such as the file the Host block is in), then its rows (FieldRow with
// density "editor"), a table or a line of fields to add one more.
export function EditorSection({ title, meta, children }: { title: string; meta?: ReactNode; children: ReactNode }) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="border-t border-line py-3.5 first:border-t-0 first:pt-0">
      <div className="mb-2.5 flex min-w-0 items-baseline gap-2">
        <h3 id={headingId} className="shrink-0 text-[13.5px] font-semibold text-ink">{title}</h3>
        {meta === undefined ? null : <span className="ml-auto min-w-0 truncate text-right font-mono text-[11.5px] text-ink-faint">{meta}</span>}
      </div>
      {children}
    </section>
  );
}

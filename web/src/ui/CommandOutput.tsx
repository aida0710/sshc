import { useLayoutEffect, useRef } from "react";

// followSlackPx is how far above the bottom the output still counts as read to
// the end, so a scroll that stops a few pixels short keeps following.
const followSlackPx = 16;

// heightClasses caps the box: a dialog keeps a few lines in view, while a page
// has the room to show an installer's output without scrolling at every line.
const heightClasses = { compact: "max-h-40", tall: "max-h-80" } as const;

// CommandOutput shows what a command writes while it runs, keeping the newest
// line in view as it grows. Once the person scrolls up to read an earlier line,
// it stays there until they scroll back to the bottom.
export function CommandOutput({ output, label, height = "compact" }: {
  output: string;
  // label names whose output this is, for screen readers.
  label: string;
  height?: keyof typeof heightClasses;
}) {
  const outputRef = useRef<HTMLPreElement>(null);
  const following = useRef(true);

  useLayoutEffect(() => {
    const element = outputRef.current;
    if (element !== null && following.current) element.scrollTop = element.scrollHeight;
  }, [output]);

  if (output === "") return null;
  return (
    <pre
      ref={outputRef}
      role="log"
      aria-label={label}
      onScroll={(event) => {
        const element = event.currentTarget;
        following.current = element.scrollHeight - element.scrollTop - element.clientHeight <= followSlackPx;
      }}
      className={`${heightClasses[height]} overflow-auto whitespace-pre-wrap break-words rounded-md bg-control p-2 font-mono text-[11px] text-ink-muted`}
    >
      {output}
    </pre>
  );
}

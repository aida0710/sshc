import { useLayoutEffect, useRef } from "react";
import { useTranslate } from "../i18n/context";

// followSlackPx is how far above the bottom the output still counts as read to
// the end, so a scroll that stops a few pixels short keeps following.
const followSlackPx = 16;

// EngineActionOutput shows what the host's sshc writes for an action, keeping
// the newest line in view as it grows. Once the person scrolls up to read an
// earlier line, it stays there until they scroll back to the bottom.
export function EngineActionOutput({ output }: { output: string }) {
  const t = useTranslate();
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
      aria-label={t("engineInfo.output")}
      onScroll={(event) => {
        const element = event.currentTarget;
        following.current = element.scrollHeight - element.scrollTop - element.clientHeight <= followSlackPx;
      }}
      className="max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-control p-2 font-mono text-[11px] text-ink-muted"
    >
      {output}
    </pre>
  );
}

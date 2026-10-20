import { useEffect, useRef, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { PanelState } from "../ui/PanelState";

// SFTPConnectionStatus stands in for the file list while a tab waits for its
// SSH connection, and after the connection failed. One line says the stage or
// the reason; below it come the lines the connection log setting asks for, as
// a terminal shows them while it connects.
export function SFTPConnectionStatus({
  tone,
  title,
  log,
  action,
}: {
  tone: "loading" | "failed";
  title: string;
  log: readonly string[];
  action?: ReactNode;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col items-center justify-center gap-2 p-4">
      <PanelState tone={tone} title={title} {...(action === undefined ? {} : { action })} className="min-h-0 py-2" />
      {log.length === 0 ? null : <ConnectionLog lines={log} />}
    </div>
  );
}

function ConnectionLog({ lines }: { lines: readonly string[] }) {
  const t = useTranslate();
  const box = useRef<HTMLPreElement>(null);
  // Keep the newest line in view, as a terminal does while it connects.
  useEffect(() => {
    const element = box.current;
    if (element !== null) element.scrollTop = element.scrollHeight;
  }, [lines]);
  return (
    <pre
      ref={box}
      tabIndex={0}
      aria-label={t("sftp.connectionLog")}
      className="min-h-0 w-full max-w-3xl overflow-auto whitespace-pre-wrap break-all rounded-md border border-line bg-term-bg p-3 text-left font-mono text-xs leading-5 text-ink-muted"
    >
      {lines.join("\n")}
    </pre>
  );
}

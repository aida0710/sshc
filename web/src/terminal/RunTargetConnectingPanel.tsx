import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { Spinner } from "../ui/Spinner";

// A reachable run target opens a session within this time; the panel is only
// shown for an open that takes longer, so it does not flash on every open.
const connectingPanelDelayMs = 600;

// RunTargetConnectingPanel covers the terminal area while a session is being
// opened on a run target whose host has not answered yet. Without it the
// screen shows nothing until the SSH connection to the host succeeds or times
// out. If it fails, RunTargetRefusalPanel takes its place with the reason.
export function RunTargetConnectingPanel({ alias }: { alias: string }) {
  const t = useTranslate();
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const timer = window.setTimeout(() => setShown(true), connectingPanelDelayMs);
    return () => window.clearTimeout(timer);
  }, []);
  if (!shown) return null;
  return (
    <div className="absolute inset-0 z-30 flex items-center justify-center bg-canvas p-4">
      <section role="status" className="flex w-full max-w-lg items-start gap-3 rounded-lg border border-line bg-card p-5 shadow-sm">
        <Spinner className="mt-0.5 size-[18px] shrink-0 text-ink-muted" />
        <div className="flex min-w-0 flex-col gap-1">
          <h2 className="break-words text-sm font-medium text-ink">{t("runTarget.connecting", { alias })}</h2>
          <p className="break-words text-sm leading-6 text-ink-muted">{t("runTarget.connectingDetail")}</p>
        </div>
      </section>
    </div>
  );
}

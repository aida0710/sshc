import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { Spinner } from "../ui/Spinner";
import { useDelayedFlag } from "../ui/useDelayedFlag";
import { waitNoticeDelayMs } from "./useRunTargetLink";

// RunTargetConnectingPanel covers the terminal area while the engine in use has not answered: while its sessions are
// read after a switch, or while one is opened on a run target. Without it the area shows nothing, or the state of an
// engine with no sessions, until the SSH connection to the host succeeds or times out. If that fails,
// RunTargetRefusalPanel takes its place with the reason.
export function RunTargetConnectingPanel({ alias, awaitingEngine, onUseThisMachine }: {
  // alias is the run target waited for; null is this machine, which has no connection to wait for.
  alias: string | null;
  // awaitingEngine says the SSH connection is there and only its engine has not answered.
  awaitingEngine: boolean;
  onUseThisMachine: () => void;
}) {
  const t = useTranslate();
  const shown = useDelayedFlag(true, waitNoticeDelayMs);
  const elapsedSeconds = useElapsedSeconds();
  if (!shown) return null;
  const text = alias === null
    ? { heading: t("runTarget.loadingThisMachine"), detail: "" }
    : awaitingEngine
      ? { heading: t("runTarget.awaitingEngine", { alias }), detail: t("runTarget.awaitingEngineDetail") }
      : { heading: t("runTarget.connecting", { alias }), detail: t("runTarget.connectingDetail") };
  return (
    <div className="absolute inset-x-0 top-0 bottom-(--mobile-navigation-inset) z-30 flex items-center justify-center bg-canvas p-4">
      <section className="flex w-full max-w-lg items-start gap-3 rounded-lg border border-line bg-card p-5 shadow-card">
        <Spinner className="mt-0.5 size-[18px] shrink-0 text-ink-muted" />
        <div className="flex min-w-0 grow flex-col gap-1">
          <div role="status" className="flex min-w-0 flex-col gap-1">
            <h2 className="break-words text-sm font-semibold text-ink">{text.heading}</h2>
            {text.detail === "" ? null : <p className="break-words text-sm leading-6 text-ink-muted">{text.detail}</p>}
          </div>
          {/* Outside the status: a screen reader would otherwise read it out every second. */}
          <p className="text-xs tabular-nums text-ink-faint">{t("runTarget.elapsed", { seconds: String(elapsedSeconds) })}</p>
          {alias === null ? null : (
            <div className="mt-2">
              <Button onClick={onUseThisMachine}>{t("runTarget.useThisMachine")}</Button>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}

const secondMs = 1_000;

function useElapsedSeconds(): number {
  const [startedAt] = useState(() => Date.now());
  const [now, setNow] = useState(startedAt);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), secondMs);
    return () => window.clearInterval(timer);
  }, []);
  return Math.floor((now - startedAt) / secondMs);
}

import type { ApiError } from "../api/client";
import { useTranslate } from "../i18n/context";

// RunTargetRefusalDetails shows what the engine answered, for telling apart
// refusals whose headings read alike or for asking someone about one.
export function RunTargetRefusalDetails({ id, refusal }: { id: string; refusal: ApiError }) {
  const t = useTranslate();
  const reason = refusal.problem?.reason;
  // The detail of an incompatible engine is its version, shown above already.
  const detail = refusal.code === "remote_engine_incompatible" ? undefined : refusal.problem?.detail;
  return (
    <dl id={id} className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-md border border-line p-3 text-xs">
      <dt className="text-ink-muted">{t("runTarget.errorCode")}</dt>
      <dd className="break-all font-mono text-ink">{reason === undefined ? refusal.code : `${refusal.code} (${reason})`}</dd>
      {detail === undefined || detail === "" ? null : (
        <>
          <dt className="text-ink-muted">{t("runTarget.errorDetail")}</dt>
          <dd className="break-all font-mono text-ink">{detail}</dd>
        </>
      )}
    </dl>
  );
}

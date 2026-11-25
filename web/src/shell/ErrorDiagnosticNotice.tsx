import { useTranslate } from "../i18n/context";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { Banner, bannerButton } from "../ui/Banner";
import { CopyButton } from "../ui/CopyButton";
import { Button } from "../ui/surface";
import type { RequestFailureDiagnostic } from "../api/client";
import { workspaceRefusalMessages } from "../api/workspaceRefusals";

export function diagnosticReport(version: string, diagnostic: RequestFailureDiagnostic): string {
  const lines = [
    `Version: ${version || "unknown"}`,
    `Code: ${diagnostic.code}`,
    `HTTP: ${diagnostic.status === 0 ? "network unavailable" : diagnostic.status}`,
    `Operation: ${diagnostic.method.toUpperCase()} ${diagnostic.path}`,
  ];
  if (diagnostic.detail !== undefined) lines.push(`Detail: ${diagnostic.detail}`);
  return lines.join("\n");
}

export function ErrorDiagnosticNotice({
  diagnostic,
  version,
  onClose,
}: {
  diagnostic: RequestFailureDiagnostic;
  version: string;
  onClose: () => void;
}) {
  const t = useTranslate();
  const report = diagnosticReport(version, diagnostic);

  return (
    <Banner
      tone="danger"
      labelledBy="request-failure-heading"
      multiline
      actions={(
        <Button small onClick={onClose} className={bannerButton}>
          {t("diagnostic.dismiss")}
        </Button>
      )}
      details={(
        <details>
          <DisclosureSummary className="text-ink-muted">{t("diagnostic.showDetails")}</DisclosureSummary>
          <pre className="mt-2 max-h-36 overflow-auto whitespace-pre-wrap break-words rounded-md border border-line bg-card p-2 text-ink">
            {report}
          </pre>
          <div className="mt-2">
            <CopyButton value={report} label="copy.diagnosticReport" />
          </div>
        </details>
      )}
    >
      <h2 id="request-failure-heading" className="text-sm font-semibold text-danger">
        {t("diagnostic.requestFailed")}
      </h2>
      <p className="mt-1 leading-5 text-ink-muted">
        {t(workspaceRefusalMessages[diagnostic.code] ?? "diagnostic.requestFailedHint", { code: diagnostic.code })}
      </p>
    </Banner>
  );
}

import { useTranslate } from "../i18n/context";
import { CopyButton } from "../ui/CopyButton";
import type { RequestFailureDiagnostic } from "../api/client";

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
    <section
      aria-labelledby="request-failure-heading"
      className="shrink-0 border-b border-danger/40 bg-danger/5 px-4 py-3"
    >
      <div className="mx-auto flex max-w-5xl flex-col gap-2">
        <div className="flex items-start gap-3">
          <div className="min-w-0 grow">
            <h2 id="request-failure-heading" className="text-sm font-semibold text-danger">
              {t("diagnostic.requestFailed")}
            </h2>
            <p className="mt-1 text-xs leading-5 text-ink-muted">
              {t("diagnostic.requestFailedHint", { code: diagnostic.code })}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("diagnostic.dismiss")}
            className="shrink-0 rounded border border-control-line px-2 py-1 text-xs text-ink-muted"
          >
            {t("diagnostic.dismiss")}
          </button>
        </div>
        <details className="text-xs">
          <summary className="cursor-pointer text-ink-muted">{t("diagnostic.showDetails")}</summary>
          <pre className="mt-2 max-h-36 overflow-auto whitespace-pre-wrap break-words rounded border border-line bg-card p-2 text-ink">
            {report}
          </pre>
          <div className="mt-2">
            <CopyButton value={report} label="copy.diagnosticReport" />
          </div>
        </details>
      </div>
    </section>
  );
}

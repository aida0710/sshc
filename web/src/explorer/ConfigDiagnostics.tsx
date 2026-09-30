import type { Diagnostic } from "../api/config";
import { useTranslate } from "../i18n/context";
import { hintText, sectionHeading } from "../ui/form";

function diagnosticTone(severity: Diagnostic["severity"]): string {
  if (severity === "error") return "text-danger";
  return severity === "warning" ? "text-notice-ink" : "text-ink-muted";
}

// ConfigDiagnostics は、Include をたどったときに見つかった問題を並べる。
export function ConfigDiagnostics({ diagnostics }: { diagnostics: Diagnostic[] }) {
  const t = useTranslate();
  const found = diagnostics.length > 0;
  return (
    <div className={`border-t border-line p-3 ${found ? "bg-notice" : "bg-toolbar"}`}>
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className={sectionHeading}>{t("explorer.diagnostics")}</h3>
        <span className={`font-mono text-xs ${found ? "text-notice-ink" : "text-ink-faint"}`}>{diagnostics.length}</span>
      </div>
      {found ? (
        <ul className="flex flex-col gap-1">
          {diagnostics.map((diagnostic, index) => (
            <li key={`${diagnostic.code}-${index}`} className={`font-mono text-xs ${diagnosticTone(diagnostic.severity)}`}>
              {`${diagnostic.code} ${diagnostic.path ?? diagnostic.absolute ?? ""}${diagnostic.line === undefined ? "" : `:${diagnostic.line}`} ${diagnostic.detail ?? ""}`}
            </li>
          ))}
        </ul>
      ) : (
        <p className={hintText}>{t("explorer.noIncludeProblem")}</p>
      )}
    </div>
  );
}

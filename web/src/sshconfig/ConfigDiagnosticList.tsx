import { useTranslate } from "../i18n/context";
import {
  configDiagnosticDetail,
  configDiagnosticPlace,
  describeConfigDiagnostic,
  describeConfigDiagnosticGroup,
  groupConfigDiagnostics,
  type ConfigDiagnostic,
} from "./configDiagnostics";

function severityTone(severity: string): string {
  if (severity === "error") return "text-danger";
  return severity === "warning" ? "text-notice-ink" : "text-ink-muted";
}

// ConfigDiagnosticList says the problems found in the SSH config as sentences. A kind that occurs more
// than once is one line with its count, which opens to the places.
export function ConfigDiagnosticList({ diagnostics, className = "" }: { diagnostics: readonly ConfigDiagnostic[]; className?: string }) {
  const t = useTranslate();
  return (
    <ul className={`flex flex-col gap-1 text-xs ${className}`}>
      {groupConfigDiagnostics(diagnostics).map((group) =>
        group.diagnostics.length === 1 ? (
          <li key={group.code} className={`break-words ${severityTone(group.severity)}`}>
            {describeConfigDiagnostic(t, group.diagnostics[0]!)}
          </li>
        ) : (
          <li key={group.code} className={severityTone(group.severity)}>
            <details>
              <summary className="cursor-pointer">{describeConfigDiagnosticGroup(t, group.code, group.diagnostics.length)}</summary>
              <ul className="mt-1 flex flex-col gap-0.5 pl-4">
                {group.diagnostics.map((diagnostic, index) => (
                  <li key={`${configDiagnosticPlace(diagnostic)}-${index}`} className="break-all font-mono text-ink-muted">
                    {[configDiagnosticPlace(diagnostic), configDiagnosticDetail(diagnostic)].filter((part) => part !== "").join("  ")}
                  </li>
                ))}
              </ul>
            </details>
          </li>
        ),
      )}
    </ul>
  );
}

import type { Diagnostic } from "../api/config";
import type { CodeMarker } from "../editor/CodeField";
import type { Translate } from "../i18n/context";
import { describeConfigDiagnostic } from "../sshconfig/configDiagnostics";

// diagnosticMarkersOf marks the lines of the file at path that the engine found a problem in (a keyword OpenSSH does
// not know, an Include that reads nothing), each with the sentence the list of problems says. A problem of no line or
// of another file marks nothing here.
export function diagnosticMarkersOf(t: Translate, diagnostics: readonly Diagnostic[], path: string | undefined): CodeMarker[] {
  if (path === undefined) return [];
  return diagnostics.flatMap((diagnostic): CodeMarker[] => {
    const line = diagnostic.line ?? 0;
    if (diagnostic.path !== path || line <= 0) return [];
    const severity = diagnostic.severity === "error" ? "error" : diagnostic.severity === "warning" ? "warning" : "info";
    return [{ line, severity, message: describeConfigDiagnostic(t, diagnostic) }];
  });
}

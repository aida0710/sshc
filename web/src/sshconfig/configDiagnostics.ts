import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

// A problem the engine found while reading the SSH config and its Includes. The SSH Config page, the
// Diagnostics page and a refused save each receive one of a few shapes of it; these fields are the ones
// all of them carry.
export type ConfigDiagnostic = {
  severity: string;
  code: string;
  path?: string;
  absolute?: string;
  line?: number;
  detail?: string;
};

// The sentence for one problem, and the heading for several of the same kind. The codes are the engine's
// (internal/config/graph.go). Each sentence says what the problem is about first and where it is last.
const sentences: Record<string, { one: MessageKey; many: MessageKey; showsDetail: boolean }> = {
  include_no_match: { one: "configDiagnostic.includeNoMatch", many: "configDiagnostic.includeNoMatchMany", showsDetail: true },
  include_unreadable: { one: "configDiagnostic.includeUnreadable", many: "configDiagnostic.includeUnreadableMany", showsDetail: true },
  include_cycle: { one: "configDiagnostic.includeCycle", many: "configDiagnostic.includeCycleMany", showsDetail: true },
  include_duplicate: { one: "configDiagnostic.includeDuplicate", many: "configDiagnostic.includeDuplicateMany", showsDetail: true },
  include_conditional: { one: "configDiagnostic.includeConditional", many: "configDiagnostic.includeConditionalMany", showsDetail: true },
  include_outside_root: { one: "configDiagnostic.includeOutsideRoot", many: "configDiagnostic.includeOutsideRootMany", showsDetail: true },
  include_depth_exceeded: { one: "configDiagnostic.includeDepthExceeded", many: "configDiagnostic.includeDepthExceededMany", showsDetail: true },
  include_unsupported_expansion: { one: "configDiagnostic.includeUnsupported", many: "configDiagnostic.includeUnsupportedMany", showsDetail: true },
  include_without_argument: { one: "configDiagnostic.includeEmpty", many: "configDiagnostic.includeEmptyMany", showsDetail: false },
  // The engine's detail here is a fixed English note the sentence already says.
  unstructured_line: { one: "configDiagnostic.unstructuredLine", many: "configDiagnostic.unstructuredLineMany", showsDetail: false },
};

// configDiagnosticPlace is where a problem is: the file and, when known, its line.
export function configDiagnosticPlace(diagnostic: ConfigDiagnostic): string {
  const file = diagnostic.path ?? diagnostic.absolute ?? "";
  return diagnostic.line === undefined || diagnostic.line <= 0 ? file : `${file}:${diagnostic.line}`;
}

// configDiagnosticDetail is the part of the engine's detail worth showing: the pattern, the file or the
// error. It is empty for a kind whose detail adds nothing to its sentence.
export function configDiagnosticDetail(diagnostic: ConfigDiagnostic): string {
  if (sentences[diagnostic.code]?.showsDetail === false) return "";
  return diagnostic.detail ?? "";
}

// describeConfigDiagnostic says one problem as a sentence. A code this screen does not know keeps the
// code itself, so that it can still be looked up.
export function describeConfigDiagnostic(t: Translate, diagnostic: ConfigDiagnostic): string {
  const where = configDiagnosticPlace(diagnostic);
  const known = sentences[diagnostic.code];
  const detail = configDiagnosticDetail(diagnostic);
  if (known === undefined) return t("configDiagnostic.unknown", { code: diagnostic.code, where, detail });
  return t(known.one, { where, detail });
}

// describeConfigDiagnosticGroup is the heading for several problems of one kind.
export function describeConfigDiagnosticGroup(t: Translate, code: string, count: number): string {
  const known = sentences[code];
  return known === undefined ? t("configDiagnostic.unknownMany", { code, count }) : t(known.many, { count });
}

export type ConfigDiagnosticGroup = { code: string; severity: string; diagnostics: ConfigDiagnostic[] };

const severityRank: Record<string, number> = { error: 0, warning: 1 };

// groupConfigDiagnostics puts problems of one kind together, the most serious kind first, keeping the
// engine's order within a kind.
export function groupConfigDiagnostics(diagnostics: readonly ConfigDiagnostic[]): ConfigDiagnosticGroup[] {
  const groups = new Map<string, ConfigDiagnosticGroup>();
  for (const diagnostic of diagnostics) {
    const group = groups.get(diagnostic.code);
    if (group === undefined) groups.set(diagnostic.code, { code: diagnostic.code, severity: diagnostic.severity, diagnostics: [diagnostic] });
    else group.diagnostics.push(diagnostic);
  }
  return [...groups.values()].sort((a, b) => (severityRank[a.severity] ?? 2) - (severityRank[b.severity] ?? 2));
}

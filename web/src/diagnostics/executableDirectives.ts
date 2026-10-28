import type { EffectiveResponse } from "../api/diagnostics";

export type ExecutableDirective = EffectiveResponse["executableDirectives"][number];

// unavoidableDirectives are the directives a connection would run that no command-line option can turn
// off. The engine refuses an authentication test or a key registration that would run one until the user
// has seen the exact commands (internal/effective Report.Unavoidable), so the screens ask for the same ones.
export function unavoidableDirectives(directives: readonly ExecutableDirective[]): ExecutableDirective[] {
  return directives.filter((directive) => directive.onConnect && !directive.overridable);
}

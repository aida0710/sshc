import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import { connectionsJumpingThrough } from "./jumpHops";

// The names of connections, as small chips in a row that wraps and scrolls when it is long.
function AliasChips({ aliases }: { aliases: readonly string[] }) {
  return (
    <ul className="flex max-h-48 flex-wrap gap-1.5 overflow-y-auto">
      {aliases.map((alias) => (
        <li key={alias} className="rounded-md bg-surface-subtle px-2 py-1 font-mono text-xs text-ink">{alias}</li>
      ))}
    </ul>
  );
}

// DeleteConnectionsBody says what deleting connections does, and which other connections go through one of them as a
// jump host: those can no longer connect once it is gone, and nothing else would tell before they fail.
export function DeleteConnectionsBody({ body, deleting, hosts, listDeleting = false }: {
  body: string;
  deleting: readonly string[];
  hosts: readonly HostEntry[];
  // listDeleting shows the deleted connections themselves, for a deletion of several.
  listDeleting?: boolean;
}) {
  const t = useTranslate();
  const dependents = connectionsJumpingThrough(hosts, deleting).map((host) => host.identity.alias);
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-ink-muted">{body}</p>
      {listDeleting ? <AliasChips aliases={deleting} /> : null}
      {dependents.length === 0 ? null : (
        <>
          <p className="text-sm text-notice-ink">{t("conn.deleteJumpDependents")}</p>
          <AliasChips aliases={dependents} />
        </>
      )}
    </div>
  );
}

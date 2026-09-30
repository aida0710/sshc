import type { KeyInventoryResponse } from "./api";
import { useTranslate } from "../i18n/context";
import { sectionHeading, tableHeadCell, tableHeadRow } from "../ui/form";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";

type AgentSort = "algorithm" | "fingerprint" | "comment";
type AgentIdentity = KeyInventoryResponse["agentIdentities"][number];

function algorithmText(identity: AgentIdentity): string {
  return identity.bits > 0 ? `${identity.algorithm} · ${identity.bits}` : identity.algorithm;
}

function agentSortValue(identity: AgentIdentity, column: AgentSort): SortValue {
  switch (column) {
    case "algorithm": return algorithmText(identity);
    case "fingerprint": return identity.fingerprint;
    case "comment": return identity.comment;
  }
}

// What the SSH agent currently holds, and the config lines that delegate to
// it. Read-only: keys are added and removed from their own rows.
export function AgentSection({ inventory }: {
  inventory: Pick<KeyInventoryResponse, "agentAvailable" | "agentIdentities" | "agentDelegations">;
}) {
  const t = useTranslate();
  const agentSort = useTableSort<AgentSort>("algorithm");
  return (
    <section aria-labelledby="agent-heading" className="flex flex-col gap-2">
      <h3 id="agent-heading" className={sectionHeading}>
        {t("keys.agentHeading")}
      </h3>
      {inventory.agentAvailable ? (
        inventory.agentIdentities.length === 0 ? (
          <p className="text-sm text-ink-muted">{t("keys.agentEmpty")}</p>
        ) : (
          <table className="w-full min-w-[32rem] text-left text-sm">
            <caption className="sr-only">
              {t("keys.agentIdentitiesCaption")}
            </caption>
            <thead>
              <tr className={tableHeadRow}>
                <SortableTableHeader column="algorithm" {...agentSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
                  {t("keys.colAlgorithm")}
                </SortableTableHeader>
                <SortableTableHeader column="fingerprint" {...agentSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
                  {t("keys.colFingerprint")}
                </SortableTableHeader>
                <SortableTableHeader column="comment" {...agentSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
                  {t("keys.colComment")}
                </SortableTableHeader>
              </tr>
            </thead>
            <tbody>
              {agentSort.sorted(inventory.agentIdentities, agentSortValue).map((identity) => (
                <tr
                  key={identity.fingerprint}
                  className="border-b border-line"
                >
                  <td className="py-2 pr-3">{algorithmText(identity)}</td>
                  <td className="py-2 pr-3 font-mono text-xs break-all">
                    {identity.fingerprint}
                  </td>
                  <td className="py-2">{identity.comment}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )
      ) : (
        <p className="text-sm text-notice-ink">
          {t("keys.agentUnavailable")}
        </p>
      )}
      {inventory.agentDelegations.length > 0 && (
        <>
          <p className="text-sm text-ink-muted">
            {t("keys.agentDelegationsNote")}
          </p>
          <ul className="text-sm text-ink-muted">
            {inventory.agentDelegations.map((reference) => (
              <li key={`${reference.configPath}:${reference.line}`}>
                {t("keys.reference", {
                  directive: reference.directive,
                  value: reference.value,
                  path: reference.configPath,
                  line: reference.line,
                })}
                {reference.hostPatterns.length > 0
                  ? ` (${reference.hostPatterns.join(" ")})`
                  : ""}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

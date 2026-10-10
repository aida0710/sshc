import { useTranslate } from "../i18n/context";
import { localHostAlias } from "./localHost";
import type { ManagedTransferJob } from "./transferManager";
import { getTransferJobRoute, type TransferEndpoint } from "./transferJobRoute";

export function TransferJobRoute({ job, compact = false }: { job: ManagedTransferJob; compact?: boolean }) {
  const t = useTranslate();
  const { source, destination } = getTransferJobRoute(job);
  function endpointLabel(endpoint: TransferEndpoint): string {
    const alias = "browser" in endpoint
      ? t(endpoint.browser === "source" ? "sftp.manager.browserSource" : "sftp.manager.browserDestination")
      : endpoint.alias === localHostAlias ? t("sftp.local.connection") : endpoint.alias;
    return compact || endpoint.path === "" ? alias : `${alias}:${endpoint.path}`;
  }
  const operation = job.operation === "delete" ? t("sftp.delete")
    : job.operation === "move" ? t("sftp.manager.operation.move")
    : job.operation === "copy" ? t("sftp.manager.operation.copy")
    : job.operation === "put" || job.direction === "upload" ? t("sftp.manager.upload") : t("sftp.manager.download");
  return (
    <span className={`block min-w-0 text-xs text-ink-muted ${compact ? "truncate" : "break-all"}`}>
      {compact ? <span className="mr-2">{operation}</span> : null}
      <span className={compact ? "" : "font-mono"}>{endpointLabel(source)}{destination === null ? "" : ` → ${endpointLabel(destination)}`}</span>
    </span>
  );
}

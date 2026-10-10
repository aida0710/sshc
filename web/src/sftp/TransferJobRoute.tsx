import { useTranslate } from "../i18n/context";
import { localHostAlias } from "./localHost";
import type { ManagedTransferJob } from "./transferManager";
import { getTransferJobRoute, type TransferEndpoint } from "./transferJobRoute";

export function TransferJobRoute({ job }: { job: ManagedTransferJob }) {
  const t = useTranslate();
  const { source, destination } = getTransferJobRoute(job);
  function endpointLabel(endpoint: TransferEndpoint): string {
    const alias = "browser" in endpoint
      ? t(endpoint.browser === "source" ? "sftp.manager.browserSource" : "sftp.manager.browserDestination")
      : endpoint.alias === localHostAlias ? t("sftp.local.connection") : endpoint.alias;
    return endpoint.path === "" ? alias : `${alias}:${endpoint.path}`;
  }
  const operation = job.operation === "delete" ? t("sftp.delete")
    : job.operation === "move" ? t("sftp.manager.operation.move")
    : job.operation === "copy" ? t("sftp.manager.operation.copy")
    : job.operation === "put" || job.direction === "upload" ? t("sftp.manager.upload") : t("sftp.manager.download");
  return (
    <span className="col-span-2 min-w-0 text-[11px] text-ink-muted">
      <span className="mr-2">{operation}</span>
      <span className="break-all font-mono">{endpointLabel(source)}{destination === null ? "" : ` → ${endpointLabel(destination)}`}</span>
    </span>
  );
}

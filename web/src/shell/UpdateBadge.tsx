import { type ReactNode } from "react";
import { updateApi, type UpdateApi } from "../api/update";
import { useTranslate } from "../i18n/context";
import { useUpdateStatus } from "./useUpdateStatus";
import { UpdateControls } from "./UpdateControls";
import { isSafeHttpURL } from "../terminal/links";

type UpdateBadgeProps = {
  api?: UpdateApi;
  current?: string;
  indicator?: ReactNode;
  enabled?: boolean;
};

export function UpdateBadge({ api = updateApi, current = "", indicator, enabled = true }: UpdateBadgeProps) {
  const t = useTranslate();
  const { status, setStatus } = useUpdateStatus(api, enabled);

  const displayedCurrent = status?.current ?? current;
  if (displayedCurrent === "") {
    return null;
  }
  return (
    <div className="border-t border-line px-2 py-2 text-xs text-ink-muted">
      <div className="flex items-center gap-2">
        {indicator}
        <p>{t("update.version", { version: displayedCurrent })}</p>
      </div>
      {status === null || !status.available || status.pageUrl === undefined || !isSafeHttpURL(status.pageUrl) ? null : (
        <p className={indicator === undefined ? "mt-1" : "mt-1 pl-3.5"}>
          <a
            href={status.pageUrl}
            target="_blank"
            rel="noreferrer noopener"
            className="text-ink underline underline-offset-2"
          >
            {t("update.available", { version: status.latest ?? "" })}
          </a>
        </p>
      )}
      {status === null ? null : <UpdateControls status={status} api={api} onStart={(job) => setStatus({ ...status, canUpdate: false, job })} />}
    </div>
  );
}

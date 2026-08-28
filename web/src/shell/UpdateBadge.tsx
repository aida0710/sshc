import { useEffect, useState } from "react";
import { integrationsApi, type IntegrationsApi, type UpdateStatus } from "../api/integrations";
import { useTranslate } from "../i18n/context";

type UpdateBadgeProps = {
  api?: IntegrationsApi;
  current?: string;
};

export function UpdateBadge({ api = integrationsApi, current = "" }: UpdateBadgeProps) {
  const t = useTranslate();
  const [status, setStatus] = useState<UpdateStatus | null>(null);

  useEffect(() => {
    let active = true;
    void api
      .updateStatus()
      .then((loaded) => {
        if (active) setStatus(loaded);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [api]);

  const displayedCurrent = status?.current ?? current;
  if (displayedCurrent === "") {
    return null;
  }
  return (
    <div className="border-t border-line px-2 py-2 text-xs text-ink-muted">
      <p>{t("update.version", { version: displayedCurrent })}</p>
      {status === null || !status.available || status.pageUrl === undefined ? null : (
        <p className="mt-1">
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
    </div>
  );
}

import { useEffect, useState, type ReactNode } from "react";
import { integrationsApi, type IntegrationsApi, type UpdateStatus } from "../api/integrations";
import { useTranslate } from "../i18n/context";

type UpdateBadgeProps = {
  api?: IntegrationsApi;
  current?: string;
  indicator?: ReactNode;
};

export function UpdateBadge({ api = integrationsApi, current = "", indicator }: UpdateBadgeProps) {
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
      <div className="flex items-center gap-2">
        {indicator}
        <p>{t("update.version", { version: displayedCurrent })}</p>
      </div>
      {status === null || !status.available || status.pageUrl === undefined ? null : (
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
    </div>
  );
}

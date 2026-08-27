import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { Icon } from "../ui/icons";
import { summarizeConnection, type ConnectionSavedState } from "./connectionSavedState";

type ConnectionSummaryProps = {
  state: ConnectionSavedState;
  dirty: boolean;
  refreshing: boolean;
  onConnect: () => void;
  connecting: boolean;
  connectAvailable?: boolean;
  onToggleManage: () => void;
  managing: boolean;
};

export function ConnectionSummary({
  state,
  dirty,
  refreshing,
  onConnect,
  connecting,
  connectAvailable = true,
  onToggleManage,
  managing,
}: ConnectionSummaryProps) {
  const t = useTranslate();
  const summary = summarizeConnection(state);
  const blocked = dirty || refreshing;
  const explicitKey = summary.privateKey.state !== "none";
  const passwordConflict = explicitKey && (
    summary.accountPassword.state === "dedicated" || summary.accountPassword.state === "named"
  );
  const reasonID = `connection-actions-${encodeURIComponent(summary.alias)}`;

  function privateKeyText() {
    switch (summary.privateKey.state) {
      case "none": return t("conn.summaryKeyNone");
      case "known": return `${summary.privateKey.path} · ${summary.privateKey.fingerprint}`;
      case "custom": return summary.privateKey.path;
      case "complex": return t("conn.summaryKeyComplex");
      case "unavailable": return t("conn.summaryKeyUnavailable", { path: summary.privateKey.path });
    }
  }

  function keyPassphraseText() {
    switch (summary.keyPassphrase.state) {
      case "none": return t("conn.summaryKeyPassphraseNone");
      case "dedicated": return t("conn.summaryKeyPassphraseDedicated");
      case "named": return t("conn.summaryKeyPassphraseNamed", { name: summary.keyPassphrase.name });
      case "not_needed": return t("conn.summaryKeyPassphraseNotNeeded");
      case "locked": return t("conn.summaryLocked");
      case "unavailable": return t("conn.summaryUnavailable");
    }
  }

  function accountPasswordText() {
    switch (summary.accountPassword.state) {
      case "none": return t("conn.summaryPasswordNone");
      case "dedicated": return t("conn.summaryPasswordDedicated");
      case "named": return t("conn.summaryPasswordNamed", { name: summary.accountPassword.name });
      case "locked": return t("conn.summaryLocked");
      case "unavailable": return t("conn.summaryUnavailable");
    }
  }

  return (
    <section data-connection-summary aria-labelledby="connection-summary-heading" className="sshc-card shrink-0 overflow-hidden rounded-md bg-card">
      <header className="flex flex-wrap items-start justify-between gap-4 px-5 py-5">
        <div className="flex min-w-0 items-start gap-3">
          <span aria-hidden="true" className="mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-lg bg-select-fill text-accent">
            <Icon name="terminal" className="size-5" />
          </span>
          <div className="min-w-0">
            <p className="text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-ink-faint">{t("conn.summarySaved")}</p>
            <h2 id="connection-summary-heading" className="truncate text-2xl font-semibold tracking-tight text-ink">
              {summary.alias}
            </h2>
            <p className="mt-1 break-all font-mono text-sm text-ink-muted">{summary.endpoint}</p>
          </div>
        </div>
        <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${dirty ? "bg-notice text-notice-ink" : "bg-select-fill text-ink-muted"}`}>
          {dirty ? t("conn.summaryUnsaved") : t("conn.summarySavedState")}
        </span>
      </header>

      <dl className="grid gap-px border-y border-line bg-line sm:grid-cols-2 lg:grid-cols-4">
        <div className="min-w-0 bg-card px-4 py-3">
          <dt className="text-[0.68rem] font-semibold uppercase tracking-wide text-ink-faint">{t("conn.summaryGroup")}</dt>
          <dd className="mt-1 truncate text-sm text-ink">{summary.group || t("conn.summaryNoGroup")}</dd>
        </div>
        <div className="min-w-0 bg-card px-4 py-3">
          <dt className="text-[0.68rem] font-semibold uppercase tracking-wide text-ink-faint">{t("conn.summaryPrivateKey")}</dt>
          <dd className="mt-1 break-all text-sm text-ink">{privateKeyText()}</dd>
        </div>
        <div className="min-w-0 bg-card px-4 py-3">
          <dt className="text-[0.68rem] font-semibold uppercase tracking-wide text-ink-faint">{t("conn.summaryKeyPassphrase")}</dt>
          <dd className="mt-1 text-sm text-ink">{keyPassphraseText()}</dd>
        </div>
        {!explicitKey ? (
          <div className="min-w-0 bg-card px-4 py-3">
            <dt className="text-[0.68rem] font-semibold uppercase tracking-wide text-ink-faint">{t("conn.summaryAccountPassword")}</dt>
            <dd className="mt-1 text-sm text-ink">{accountPasswordText()}</dd>
          </div>
        ) : passwordConflict ? (
          <div className="min-w-0 bg-notice px-4 py-3">
            <dt className="text-[0.68rem] font-semibold uppercase tracking-wide text-notice-ink">{t("conn.summaryAccountPassword")}</dt>
            <dd className="mt-1 text-sm text-notice-ink">{t("conn.summaryPasswordCleanup")}</dd>
          </div>
        ) : <div aria-hidden="true" className="hidden bg-card lg:block" />}
      </dl>

      <div className="flex flex-wrap items-center gap-2 px-4 py-3">
        <Button
          kind="primary"
          disabled={blocked || connecting || !connectAvailable}
          aria-describedby={blocked ? reasonID : undefined}
          onClick={onConnect}
        >
          {connecting ? t("conn.opening") : t("conn.connect")}
        </Button>
        <Button aria-expanded={managing} onClick={onToggleManage}>
          {t("conn.manage")}
        </Button>
        {blocked ? (
          <p id={reasonID} className="w-full text-xs text-notice-ink sm:ml-auto sm:w-auto">
            {dirty ? t("conn.summaryDraftBlocksActions") : t("conn.summaryRefreshing")}
          </p>
        ) : null}
      </div>
    </section>
  );
}

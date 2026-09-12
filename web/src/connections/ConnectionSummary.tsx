import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { Icon } from "../ui/icons";
import { OperatingSystemIcon } from "../ui/OperatingSystemIcon";
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
  const showPassphrase = explicitKey && summary.keyPassphrase.state !== "not_needed";
  const showAccountPassword = !explicitKey && summary.accountPassword.state !== "none";
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
    <section data-connection-summary aria-labelledby="connection-summary-heading" className="shrink-0 border-b border-line pb-4">
      <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <OperatingSystemIcon os={state.detail.metadata.os || state.detail.metadata.detectedOS} compact />
            <h2 id="connection-summary-heading" className="min-w-0 truncate text-xl font-semibold tracking-tight text-ink sm:text-2xl">{summary.alias}</h2>
            {dirty ? <span className="rounded bg-notice px-2 py-1 text-xs font-medium text-notice-ink">{t("conn.summaryUnsaved")}</span> : null}
          </div>
          <p className="mt-1 break-all font-mono text-sm text-ink-muted">{summary.endpoint}</p>
          {summary.group === "" ? null : <p className="mt-1 flex items-center gap-1.5 text-xs text-ink-muted"><Icon name="groups" className="size-3.5" /><span className="sr-only">{t("conn.summaryGroup")}: </span>{summary.group}</p>}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button kind="primary" disabled={blocked || connecting || !connectAvailable} aria-describedby={blocked ? reasonID : undefined} onClick={onConnect} className="inline-flex items-center gap-2">
            <Icon name="terminal" className="size-4" />
            {connecting ? t("conn.opening") : t("conn.connect")}
          </Button>
          <Button aria-label={t("conn.manage")} title={t("conn.manage")} aria-expanded={managing} onClick={onToggleManage} className={`flex size-9 items-center justify-center px-2 ${managing ? "border-accent bg-select-fill text-accent" : ""}`}>
            <Icon name="moreHorizontal" className="size-4" />
          </Button>
        </div>
      </header>
      <dl className="mt-3 flex flex-wrap gap-x-5 gap-y-2 text-xs text-ink-muted">
        <div className="min-w-0">
          <dt className={explicitKey ? "mb-0.5 font-medium text-ink-muted" : "sr-only"}>{t("conn.summaryPrivateKey")}</dt>
          <dd className="break-all">{privateKeyText()}</dd>
        </div>
        {showPassphrase ? <div className="min-w-0">
          <dt className="mb-0.5 font-medium text-ink-muted">{t("conn.summaryKeyPassphrase")}</dt>
          <dd>{keyPassphraseText()}</dd>
        </div> : null}
        {showAccountPassword ? <div className="min-w-0">
          <dt className="mb-0.5 font-medium text-ink-muted">{t("conn.summaryAccountPassword")}</dt>
          <dd>{accountPasswordText()}</dd>
        </div> : null}
      </dl>
      {passwordConflict ? <p role="status" className="mt-3 rounded border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{t("conn.summaryPasswordCleanup")}</p> : null}
      {blocked ? <p id={reasonID} className="mt-3 text-xs text-notice-ink">{dirty ? t("conn.summaryDraftBlocksActions") : t("conn.summaryRefreshing")}</p> : null}
    </section>
  );
}

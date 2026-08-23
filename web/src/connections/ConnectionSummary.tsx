import { useTranslate } from "../i18n/context";
import { Button, Card, Row } from "../ui/surface";
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
    <section aria-labelledby="connection-summary-heading" className="flex flex-col gap-3">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-ink-muted">{t("conn.summarySaved")}</p>
          <h2 id="connection-summary-heading" className="truncate text-xl font-semibold text-ink">
            {summary.alias}
          </h2>
          <p className="mt-0.5 break-all font-mono text-sm text-ink-muted">{summary.endpoint}</p>
        </div>
        <span className={`rounded-full px-2 py-1 text-xs ${dirty ? "bg-notice text-notice-ink" : "bg-control text-ink-muted"}`}>
          {dirty ? t("conn.summaryUnsaved") : t("conn.summarySavedState")}
        </span>
      </header>

      <Card>
        <Row label={t("conn.summaryGroup")}>
          <p className="text-sm text-ink">{summary.group || t("conn.summaryNoGroup")}</p>
        </Row>
        <Row label={t("conn.summaryPrivateKey")}>
          <p className="break-all text-sm text-ink">{privateKeyText()}</p>
        </Row>
        <Row label={t("conn.summaryKeyPassphrase")}>
          <p className="text-sm text-ink">{keyPassphraseText()}</p>
        </Row>
        {!explicitKey ? (
          <Row label={t("conn.summaryAccountPassword")}>
            <p className="text-sm text-ink">{accountPasswordText()}</p>
          </Row>
        ) : passwordConflict ? (
          <Row label={t("conn.summaryAccountPassword")}>
            <p className="text-sm text-notice-ink">{t("conn.summaryPasswordCleanup")}</p>
          </Row>
        ) : null}
      </Card>

      <div className="flex flex-wrap items-center gap-2 rounded-xl border border-line bg-card p-3 shadow-sm">
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
          <p id={reasonID} className="w-full text-xs text-notice-ink">
            {dirty ? t("conn.summaryDraftBlocksActions") : t("conn.summaryRefreshing")}
          </p>
        ) : null}
      </div>
    </section>
  );
}

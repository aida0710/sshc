import { useEffect, useState } from "react";
import {
  integrationsApi,
  type AuthenticationResponse,
  type ConfigCheckResponse,
  type EffectiveResponse,
  type IntegrationsApi,
  type ReachabilityResponse,
} from "../api/integrations";
import {
  Field,
  control,
  hintText,
  sectionHeading,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { useTranslate } from "../i18n/context";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

type DiagnosticsPanelProps = {
  api?: IntegrationsApi;
  hosts?: string[];
  host?: string;
};

export function DiagnosticsPanel({ api = integrationsApi, host, hosts = [] }: DiagnosticsPanelProps) {
  const t = useTranslate();
  const embedded = host !== undefined;
  const [typedAlias, setTypedAlias] = useState("");
  const alias = host ?? typedAlias;
  const [config, setConfig] = useState<ConfigCheckResponse | null>(null);
  const [effective, setEffective] = useState<EffectiveResponse | null>(null);
  const [reach, setReach] = useState<ReachabilityResponse | null>(null);
  const [auth, setAuth] = useState<AuthenticationResponse | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (embedded) return;
    let active = true;
    void api
      .configCheck()
      .then((result) => {
        if (active) setConfig(result);
      })
      .catch(() => {
        if (active) setError(t("diag.configUnreadable"));
      });
    return () => {
      active = false;
    };
  }, [api, embedded, t]);

  useEffect(() => {
    setEffective(null);
    setReach(null);
    setAuth(null);
    setError("");
  }, [alias]);

  async function run<T>(operation: () => Promise<T>, apply: (value: T) => void, failure: string) {
    setError("");
    setBusy(true);
    try {
      apply(await operation());
    } catch {
      setError(failure);
    } finally {
      setBusy(false);
    }
  }

  const directives = effective?.executableDirectives ?? [];

  const checks: { label: string; start: () => void }[] = [
    {
      label: t("diag.explain"),
      start: () => void run(() => api.effective(alias), setEffective, t("diag.explainFailed")),
    },
    {
      label: t("diag.checkReachability"),
      start: () => void run(() => api.reachability(alias), setReach, t("diag.reachabilityFailed")),
    },
    {
      label: t("diag.testAuthentication"),
      start: () =>
        void run(
          () => api.authentication(alias, directives.some((directive) => !directive.overridable)),
          setAuth,
          t("diag.authenticationFailed"),
        ),
    },
  ];
  const blocked = busy || alias === "";
  const hasResults = effective !== null || reach !== null || auth !== null;

  return (
    <section
      aria-label={embedded ? t("diag.forHost", { host: host ?? "" }) : undefined}
      className={`${embedded ? "flex flex-col gap-4" : "mx-auto flex w-full max-w-5xl flex-col gap-6"} ${mobileTouchTargets}`}
    >
      {embedded ? null : (
        <PageHeader title={t("diag.heading")} description={t("diag.pageDescription")} />
      )}

      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}

      <section className="sshc-card overflow-hidden rounded-md bg-card" aria-label={t("diag.heading")}>
        <div className="flex flex-col gap-4 bg-toolbar px-4 py-4 sm:px-5">
          <div className="flex flex-wrap items-end gap-3">
            {embedded ? (
              <div className="min-w-0 flex-1">
                <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">{t("diag.hostAlias")}</p>
                <p className="mt-1 truncate font-mono text-base font-semibold text-ink">{alias}</p>
              </div>
            ) : (
              <div className="min-w-56 flex-1 sm:max-w-sm">
                <Field label={t("diag.hostAlias")}>
                  <input
                    value={typedAlias}
                    onChange={(event) => setTypedAlias(event.target.value)}
                    list="diagnostic-host-options"
                    placeholder="bastion"
                    className={`${control} font-mono`}
                  />
                </Field>
                <datalist id="diagnostic-host-options">
                  {hosts.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
                </datalist>
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              {checks.map((check, index) => (
                <Button
                  key={check.label}
                  kind={index === 0 ? "primary" : "secondary"}
                  onClick={check.start}
                  disabled={blocked}
                >
                  {check.label}
                </Button>
              ))}
            </div>
          </div>
          <p aria-live="polite" className="flex items-center gap-2 text-xs text-ink-muted">
            <span className={`h-2 w-2 rounded-full ${busy ? "bg-notice-ink" : alias === "" ? "bg-ink-faint" : "bg-live"}`} />
            {busy ? t("diag.running") : alias === "" ? t("diag.needsAlias") : t("diag.idle")}
          </p>
        </div>
      </section>

      {config ? (
        <section className="sshc-card overflow-hidden rounded-md bg-card">
          <header className="flex items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
            <div className="flex items-center gap-2">
              <Icon name="config" className="h-4 w-4 text-ink-muted" />
              <h3 className={sectionHeading}>{t("diag.configuration")}</h3>
            </div>
            <span className="rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-ink-muted">{config.files.length}</span>
          </header>
          <ul className="divide-y divide-hairline px-4">
            {config.files.map((file) => (
              <li key={file.path} className="flex items-center gap-2 py-2.5 font-mono text-xs text-ink-muted">
                <span className={`h-1.5 w-1.5 rounded-full ${file.missing ? "bg-notice-ink" : "bg-live"}`} />
                {file.path}
                {file.missing ? <span className="text-notice-ink">{t("diag.missingSuffix")}</span> : null}
              </li>
            ))}
          </ul>
          {config.diagnostics.length > 0 ? (
            <ul className="border-t border-line bg-surface-subtle px-4 py-3">
              {config.diagnostics.map((diagnostic, index) => (
                <li
                  key={`${diagnostic.code}-${index}`}
                  className={`py-1 font-mono text-xs ${
                    diagnostic.severity === "error"
                      ? "text-danger"
                      : diagnostic.severity === "warning"
                        ? "text-notice-ink"
                        : "text-ink-muted"
                  }`}
                >
                  {`${diagnostic.code} ${diagnostic.path}${diagnostic.line > 0 ? `:${diagnostic.line}` : ""} ${diagnostic.detail}`}
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      ) : null}

      {hasResults ? <div className="sshc-card overflow-hidden rounded-md bg-card">
      {directives.length > 0 ? (
        <section className="border-b border-notice-line bg-notice px-4 py-4 text-sm">
          <h3 className="font-medium text-notice-ink">{t("diag.canRunCommand")}</h3>
          <p className="text-ink-muted">{effective?.tokenWarning}</p>
          <ul className="mt-2 flex flex-col gap-1">
            {directives.map((directive) => (
              <li key={`${directive.path}:${directive.line}:${directive.keyword}`}>
                <span className="text-ink-muted">
                  {t("diag.directiveAt", {
                    keyword: directive.keyword,
                    path: directive.path,
                    line: directive.line,
                  })}
                </span>
                <pre className="whitespace-pre-wrap break-all text-ink">{directive.command}</pre>
              </li>
            ))}
          </ul>
        </section>
      ) : null}


      {effective && effective.sources.length > 0 ? (
        <section className="px-4 py-4">
          <p className="mb-2 flex items-center justify-end gap-1 text-xs text-ink-muted md:hidden">
            {t("diag.tableScrollHint")}
            <span aria-hidden="true">→</span>
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">

              <caption className={`mb-2 text-left ${hintText}`}>{t("diag.sourcesCaption")}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnKeyword")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnValue")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnWhere")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnCondition")}</th>
                  <th scope="col" className={tableHeadCell}>{t("diag.columnState")}</th>
                </tr>
              </thead>
              <tbody>
                {effective.sources.map((source) => (
                  <tr
                    key={`${source.path}:${source.line}:${source.keyword}`}
                    className={`border-b border-line ${source.winner ? "" : "opacity-60"}`}
                  >
                    <th scope="row" className="py-1.5 pr-3 text-left font-normal text-ink-muted">
                      {source.keyword}
                    </th>
                    <td className="py-1.5 pr-3 font-mono text-xs text-ink">{source.value}</td>
                    <td className="py-1.5 pr-3 font-mono text-xs text-ink-faint">{`${source.path}:${source.line}`}</td>
                    <td className="py-1.5 pr-3 font-mono text-xs text-ink-faint">{source.condition}</td>
                    <td className="py-1.5 text-ink-faint">{source.winner ? t("diag.inEffect") : t("diag.superseded")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}


      {effective && effective.route.length > 0 ? (
        <section className="border-t border-line px-4 py-4 text-sm">
          <div className="mb-3 flex items-center gap-2">
            <Icon name="connections" className="h-4 w-4 text-ink-muted" />
            <h3 className={sectionHeading}>{t("diag.route")}</h3>
          </div>
          <ol className="flex flex-col gap-2">
            {effective.route.map((stage) => (
              <li key={`${stage.order}-${stage.hop}`} className="flex items-start gap-3" style={{ marginInlineStart: `${stage.depth}rem` }}>
                <span className="mt-1 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-select-fill font-mono text-[10px] text-ink-muted">{stage.order + 1}</span>
                <div><span className="font-medium text-ink">{stage.hop}</span>
                {stage.complex ? (
                  <span className="ml-2 text-notice-ink">{t("diag.hopComplex")}</span>
                ) : (
                  <span className="ml-2 text-ink-muted">
                    {`${stage.user === "" ? "" : `${stage.user}@`}${stage.hostname}${
                      stage.port === "" ? "" : `:${stage.port}`
                    }`}
                  </span>
                )}
                {stage.parent === "" ? null : (
                  <span className="ml-2 text-ink-faint">{t("diag.reachedThrough", { parent: stage.parent })}</span>
                )}
                </div>
              </li>
            ))}
          </ol>
        </section>
      ) : null}


      {effective && effective.complexities.length > 0 ? (
        <section className="border-t border-notice-line bg-notice px-4 py-4 text-sm">
          <h3 className="font-medium text-notice-ink">{t("diag.notSimple")}</h3>
          <p className="text-ink-muted">{t("diag.notSimpleDetail")}</p>
          <ul className="mt-2 flex flex-col gap-1">
            {effective.complexities.map((note, index) => (
              <li key={`${note.code}-${note.path}-${note.line}-${index}`}>
                <span className="text-ink">{note.code}</span>
                <span className="ml-2 text-ink-muted">{`${note.path}:${note.line}`}</span>
                {note.condition === "" ? null : <span className="ml-2 text-ink-faint">{t("diag.inside", { condition: note.condition })}</span>}
                {note.detail === "" ? null : <p className="text-ink-muted">{note.detail}</p>}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {reach || auth ? <div className="grid border-t border-line md:grid-cols-2 md:divide-x md:divide-line">
        {reach ? (
          <section className="flex flex-col gap-2 px-4 py-4 text-sm">
            <div className="flex items-center justify-between gap-3">
              <h3 className={sectionHeading}>{t("diag.reachability")}</h3>
              <span className="h-2 w-2 rounded-full bg-live" />
            </div>
            <p className="font-mono text-xs text-ink">{reach.address}</p>
            <p className="font-medium text-ink">{reach.outcome}</p>
            <p className={hintText}>{reach.notice}</p>
          </section>
        ) : null}

        {auth ? (
          <section className="flex flex-col gap-2 border-t border-line px-4 py-4 text-sm md:border-t-0">
            <div className="flex items-center justify-between gap-3">
              <h3 className={sectionHeading}>{t("diag.authentication")}</h3>
              <span className={`h-2 w-2 rounded-full ${auth.authenticated ? "bg-live" : "bg-notice-ink"}`} />
            </div>
            <p className="font-medium text-ink">{auth.outcome}</p>
            {auth.method ? (
              <p className={hintText}>{t("diag.authenticationMethod", { method: auth.method })}</p>
            ) : null}
            {auth.detail ? (
              <pre className="whitespace-pre-wrap break-all font-mono text-xs text-ink-muted">{auth.detail}</pre>
            ) : null}
          </section>
        ) : null}
      </div> : null}
      </div> : null}

    </section>
  );
}

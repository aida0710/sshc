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
  sectionCard,
  sectionHeading,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { useTranslate } from "../i18n/context";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";

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

  return (
    <section
      aria-label={embedded ? t("diag.forHost", { host: host ?? "" }) : undefined}
      className={embedded ? "flex flex-col gap-4" : "mx-auto flex w-full max-w-5xl flex-col gap-6"}
    >
      {embedded ? null : (
        <PageHeader title={t("diag.heading")} description={t("diag.pageDescription")} />
      )}

      <p aria-live="polite" className={hintText}>
        {busy ? t("diag.running") : alias === "" ? t("diag.needsAlias") : t("diag.idle")}
      </p>
      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}

      <div className="flex flex-wrap items-end gap-2">
        {embedded ? null : (
          <div className="w-56">
            <Field label={t("diag.hostAlias")}>
              <input
                value={typedAlias}
                onChange={(event) => setTypedAlias(event.target.value)}
                list="diagnostic-host-options"
                placeholder="bastion"
                className={control}
              />
            </Field>
            <datalist id="diagnostic-host-options">
              {hosts.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
            </datalist>
          </div>
        )}
        {checks.map((check) => (
          <Button
            key={check.label}
            onClick={check.start}
            disabled={blocked}
          >
            {check.label}
          </Button>
        ))}
      </div>

      {config ? (
        <div className={sectionCard}>
          <h3 className={sectionHeading}>{t("diag.configuration")}</h3>
          <ul className="flex flex-col gap-1">
            {config.files.map((file) => (
              <li key={file.path} className="font-mono text-xs text-ink-muted">
                {file.path}
                {file.missing ? <span className="text-notice-ink">{t("diag.missingSuffix")}</span> : null}
              </li>
            ))}
          </ul>
          {config.diagnostics.length > 0 ? (
            <ul className="flex flex-col gap-1">
              {config.diagnostics.map((diagnostic, index) => (
                <li
                  key={`${diagnostic.code}-${index}`}
                  className={`font-mono text-xs ${
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
        </div>
      ) : null}

      {directives.length > 0 ? (
        <div className="rounded border border-notice-line p-3 text-sm">
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
        </div>
      ) : null}


      {effective && effective.sources.length > 0 ? (
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
      ) : null}


      {effective && effective.route.length > 0 ? (
        <div className={`${sectionCard} text-sm`}>
          <h3 className={sectionHeading}>{t("diag.route")}</h3>
          <ol className="flex flex-col gap-1">
            {effective.route.map((stage) => (
              <li key={`${stage.order}-${stage.hop}`} style={{ marginInlineStart: `${stage.depth}rem` }}>
                <span className="text-ink">{stage.hop}</span>
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
              </li>
            ))}
          </ol>
        </div>
      ) : null}


      {effective && effective.complexities.length > 0 ? (
        <div className="rounded border border-notice-line p-3 text-sm">
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
        </div>
      ) : null}

      {reach ? (
        <div className={`${sectionCard} text-sm`}>
          <h3 className={sectionHeading}>{t("diag.reachability")}</h3>

          <p className="font-mono text-xs text-ink">{reach.address}</p>
          <p className="text-ink">{reach.outcome}</p>
          <p className={hintText}>{reach.notice}</p>
        </div>
      ) : null}

      {auth ? (
        <div className={`${sectionCard} text-sm`}>
          <h3 className={sectionHeading}>{t("diag.authentication")}</h3>
          <p className="text-ink">{auth.outcome}</p>
          {auth.method ? (
            <p className={hintText}>{t("diag.authenticationMethod", { method: auth.method })}</p>
          ) : null}
          {auth.detail ? (
            <pre className="whitespace-pre-wrap break-all font-mono text-xs text-ink-muted">{auth.detail}</pre>
          ) : null}
        </div>
      ) : null}

    </section>
  );
}

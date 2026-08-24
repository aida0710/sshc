import { useEffect, useState } from "react";
import type {
  AuthenticationResponse,
  EffectiveResponse,
  IntegrationsApi,
  ReachabilityResponse,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { hintText, sectionHeading } from "../ui/form";
import { Button, Notice } from "../ui/surface";

type ChecksApi = Pick<IntegrationsApi, "effective" | "reachability" | "authentication">;

type ConnectionChecksProps = {
  alias: string;
  api: ChecksApi;
  disabled: boolean;
  resetKey: string | number;
};

export function ConnectionChecks({ alias, api, disabled, resetKey }: ConnectionChecksProps) {
  const t = useTranslate();
  const [reachability, setReachability] = useState<ReachabilityResponse | null>(null);
  const [authentication, setAuthentication] = useState<AuthenticationResponse | null>(null);
  const [pendingDirectives, setPendingDirectives] = useState<EffectiveResponse["executableDirectives"]>([]);
  const [reachabilityError, setReachabilityError] = useState("");
  const [authenticationError, setAuthenticationError] = useState("");
  const [busy, setBusy] = useState<"reachability" | "authentication" | null>(null);

  useEffect(() => {
    setReachability(null);
    setAuthentication(null);
    setPendingDirectives([]);
    setReachabilityError("");
    setAuthenticationError("");
    setBusy(null);
  }, [alias, resetKey]);

  async function checkReachability() {
    if (disabled || busy !== null) return;
    setBusy("reachability");
    setReachabilityError("");
    try {
      setReachability(await api.reachability(alias));
    } catch {
      setReachability(null);
      setReachabilityError(t("diag.reachabilityFailed"));
    } finally {
      setBusy(null);
    }
  }

  async function authenticate(acknowledgeExecutable: boolean) {
    setBusy("authentication");
    setAuthenticationError("");
    try {
      setAuthentication(await api.authentication(alias, acknowledgeExecutable));
      setPendingDirectives([]);
    } catch {
      setAuthentication(null);
      setAuthenticationError(t("diag.authenticationFailed"));
    } finally {
      setBusy(null);
    }
  }

  async function checkAuthentication() {
    if (disabled || busy !== null) return;
    setBusy("authentication");
    setAuthenticationError("");
    setPendingDirectives([]);
    try {
      const inspection = await api.effective(alias);
      if (inspection.executableDirectives.length > 0) {
        setPendingDirectives(inspection.executableDirectives);
        return;
      }
      setAuthentication(await api.authentication(alias, false));
    } catch {
      setAuthentication(null);
      setAuthenticationError(t("diag.authenticationFailed"));
    } finally {
      setBusy(null);
    }
  }

  const blocked = disabled || busy !== null;

  return (
    <section aria-label={t("conn.checksLabel")} className="flex flex-col gap-3 rounded-lg bg-tree px-4 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={blocked} onClick={() => void checkReachability()}>
          {busy === "reachability" ? t("conn.checking") : t("conn.checkReachability")}
        </Button>
        <Button disabled={blocked} onClick={() => void checkAuthentication()}>
          {busy === "authentication" ? t("conn.checking") : t("conn.checkAuthentication")}
        </Button>
        {disabled ? <p className={`w-full ${hintText}`}>{t("conn.summaryDraftBlocksActions")}</p> : null}
      </div>

      {reachabilityError === "" ? null : <Notice tone="danger">{reachabilityError}</Notice>}
      {authenticationError === "" ? null : <Notice tone="danger">{authenticationError}</Notice>}

      {pendingDirectives.length === 0 ? null : (
        <div className="rounded border border-notice-line bg-notice p-3 text-sm">
          <h3 className="font-medium text-notice-ink">{t("conn.checksExecutableHeading")}</h3>
          <p className={hintText}>{t("conn.checksExecutableHint")}</p>
          <ul className="mt-2 flex flex-col gap-2">
            {pendingDirectives.map((directive) => (
              <li key={`${directive.path}:${directive.line}:${directive.keyword}`}>
                <p className="text-ink-muted">
                  {t("conn.checksDirectiveAt", {
                    keyword: directive.keyword,
                    path: directive.path,
                    line: directive.line,
                  })}
                </p>
                <pre className="whitespace-pre-wrap break-all text-xs text-ink">{directive.command}</pre>
              </li>
            ))}
          </ul>
          <Button className="mt-3" disabled={busy !== null} onClick={() => void authenticate(true)}>
            {t("conn.checksAcknowledge")}
          </Button>
        </div>
      )}

      {reachability === null && authentication === null ? null : (
        <div className="sshc-card grid overflow-hidden rounded-lg bg-card sm:grid-cols-2 sm:divide-x sm:divide-line">
          {reachability === null ? null : (
            <div className="flex flex-col gap-1.5 p-3 text-sm">
              <h3 className={sectionHeading}>{t("diag.reachability")}</h3>
              <p className="font-mono text-xs text-ink">{reachability.address}</p>
              <p className="font-medium text-ink">{reachability.outcome}</p>
              <p className={hintText}>{reachability.notice}</p>
            </div>
          )}
          {authentication === null ? null : (
            <div className="flex flex-col gap-1.5 border-t border-line p-3 text-sm sm:border-t-0">
              <h3 className={sectionHeading}>{t("diag.authentication")}</h3>
              <p className="font-medium text-ink">{authentication.outcome}</p>
              {authentication.method === "" ? null : (
                <p className={hintText}>{t("diag.authenticationMethod", { method: authentication.method })}</p>
              )}
              {authentication.detail === "" ? null : (
                <pre className="whitespace-pre-wrap break-all font-mono text-xs text-ink-muted">
                  {authentication.detail}
                </pre>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

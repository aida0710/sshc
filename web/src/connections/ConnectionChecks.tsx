import { useEffect, useState } from "react";
import type { AuthenticationResponse, DiagnosticsApi, ReachabilityResponse } from "../api/diagnostics";
import { useTranslate } from "../i18n/context";
import { describeAuthentication, describeReachability, reachabilityNoticeKey } from "../diagnostics/diagnosticLabels";
import { ExecutableDirectiveConfirmation } from "../diagnostics/ExecutableDirectiveConfirmation";
import { unavoidableDirectives, type ExecutableDirective } from "../diagnostics/executableDirectives";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { hintText } from "../ui/form";
import { Icon } from "../ui/icons";
import { Button } from "../ui/surface";

type ChecksApi = Pick<DiagnosticsApi, "effective" | "reachability" | "authentication">;

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
  const [pendingDirectives, setPendingDirectives] = useState<ExecutableDirective[]>([]);
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
      const unavoidable = unavoidableDirectives((await api.effective(alias)).executableDirectives);
      if (unavoidable.length > 0) {
        setPendingDirectives(unavoidable);
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
  const hasResult = reachability !== null || authentication !== null;

  return (
    <section aria-label={t("conn.checksLabel")} className="flex flex-col gap-1.5 pt-2.5">
      <div className="flex flex-wrap items-center gap-2">
        <Button small disabled={blocked} onClick={() => void checkReachability()} className="max-w-full whitespace-normal text-left">
          {busy === "reachability" ? t("conn.checking") : t("conn.checkReachability")}
        </Button>
        <Button small disabled={blocked} onClick={() => void checkAuthentication()} className="max-w-full whitespace-normal text-left">
          {busy === "authentication" ? t("conn.checking") : t("conn.checkAuthentication")}
        </Button>
      </div>

      {reachabilityError === "" ? null : <CheckLine passed={false} text={reachabilityError} />}
      {authenticationError === "" ? null : <CheckLine passed={false} text={authenticationError} />}

      {pendingDirectives.length === 0 ? null : (
        <ExecutableDirectiveConfirmation directives={pendingDirectives} busy={busy === "authentication"} onAcknowledge={() => void authenticate(true)} />
      )}

      {reachability === null ? null : (
        <CheckLine
          passed={reachability.outcome === "reached"}
          text={t("conn.checkResult", { check: t("diag.reachability"), result: describeReachability(t, reachability.outcome) })}
          detail={reachability.address}
        />
      )}
      {authentication === null ? null : (
        <CheckLine
          passed={authentication.outcome === "authenticated"}
          text={t("conn.checkResult", { check: t("diag.authentication"), result: describeAuthentication(t, authentication.outcome) })}
          detail={authentication.method === "" ? "" : t("diag.authenticationMethod", { method: authentication.method })}
        />
      )}
      {/* 結果の読み方（直接つないだので踏み台を使っていない、など）と、認証の生の出力は、開いたときだけ見せる。 */}
      {!hasResult ? null : (
        <details className="text-xs text-ink-muted">
          <DisclosureSummary className="py-0.5 text-xs text-ink-muted">{t("conn.checksDetails")}</DisclosureSummary>
          <div className="mt-1.5 flex flex-col gap-1.5">
            {reachability === null ? null : <p className={hintText}>{t(reachabilityNoticeKey(reachability.outcome))}</p>}
            {authentication === null || authentication.detail === "" ? null : (
              <pre className="whitespace-pre-wrap break-all font-mono text-xs text-ink-muted">{authentication.detail}</pre>
            )}
          </div>
        </details>
      )}
    </section>
  );
}

// CheckLine は、確認の結果の1行：通ったかの印、結果の文、宛先や認証の方法。
function CheckLine({ passed, text, detail = "" }: { passed: boolean; text: string; detail?: string }) {
  return (
    <p role="status" className={`flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs leading-[18px] ${passed ? "text-success" : "text-notice-ink"}`}>
      <span className="flex items-baseline gap-1.5">
        <Icon name={passed ? "check" : "warning"} className="size-3.5 shrink-0 translate-y-0.5" />
        {text}
      </span>
      {detail === "" ? null : <span className="font-mono text-[11.5px] text-ink-faint">{detail}</span>}
    </p>
  );
}

import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { CopyButton } from "../ui/CopyButton";
import type { RemoteKeyPlan } from "./api";

// 1台ぶんの登録内容。どのユーザーのどのホストへ、どの行を、どのコマンドで足すかを見せる。
// sshc から登録できないホストでは、手で行う手順を添える。

const valuesFromLabels: Record<string, MessageKey> = {
  engine: "rk.valuesFromEngine",
  "ssh -G": "rk.valuesFromSshG",
};

export function RemoteKeyPlanCard({ plan, manual }: { plan: RemoteKeyPlan; manual: boolean }) {
  const t = useTranslate();
  return (
    <article aria-label={t("rk.planFor", { alias: plan.alias })}>
      <h4 className="border-b border-hairline py-3 font-mono font-semibold text-ink">{plan.alias}</h4>
      <div className="grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
        <dl className="border-b border-line lg:border-b-0 lg:border-r">
          {[
            [t("rk.effectiveUser"), plan.user === "" ? t("rk.noUser") : plan.user],
            [t("rk.destination"), `${plan.hostname}:${plan.port}`],
            [
              t("rk.valuesCameFrom"),
              plan.valuesFrom in valuesFromLabels ? t(valuesFromLabels[plan.valuesFrom]!) : plan.valuesFrom,
            ],
            [t("rk.keyFile"), plan.keyPath],
            [t("rk.fingerprint"), plan.fingerprint],
          ].map(([caption, value]) => (
            <div key={caption} className="grid gap-1 border-t border-hairline py-3 pr-5 first:border-t-0 sm:grid-cols-[9rem_minmax(0,1fr)]">
              <dt className="text-xs font-medium text-ink-muted">{caption}</dt>
              <dd className="min-w-0 break-all font-mono text-xs">{value}</dd>
            </div>
          ))}
        </dl>
        <div className="py-5 lg:pl-5">
          <p>{t("rk.appendTo", {
            remotePath: plan.remotePath,
            account: plan.user === "" ? t("rk.theRemoteAccount") : t("rk.usersAccount", { user: plan.user }),
            hostname: plan.hostname,
          })}</p>
          <pre aria-label={t("rk.keyLineLabel")} className="mt-3 overflow-x-auto rounded-lg border border-line bg-canvas p-3 font-mono text-xs">
            {plan.keyLine}
          </pre>
          <div className="mt-2"><CopyButton value={plan.keyLine} label="copy.keyLine" /></div>
          <p className="mt-5 text-ink-muted">{t("rk.remoteRuns")}</p>
          <pre aria-label={t("rk.remoteCommandLabel")} className="mt-2 overflow-x-auto rounded-lg border border-line bg-canvas p-3 font-mono text-xs">
            {plan.routine}
          </pre>
          <div className="mt-1"><CopyButton value={plan.routine} label="copy.remoteCommand" /></div>
          {manual ? (
            <div className="mt-4 rounded-[10px] bg-well p-3">
              <h5 className="font-medium text-notice-ink">{t("rk.manualHeading")}</h5>
              <ol className="mt-1 list-decimal pl-5">
                {plan.manual.map((step) => <li key={step}>{step}</li>)}
              </ol>
            </div>
          ) : null}
        </div>
      </div>
    </article>
  );
}

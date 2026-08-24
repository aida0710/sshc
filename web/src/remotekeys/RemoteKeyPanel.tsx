import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { keysApi, type KeyItem, type KeysApi } from "../keys/api";
import { useTranslate, type Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import {
  remoteKeysApi,
  type RemoteKeyInput,
  type RemoteKeyPlan,
  type RemoteKeyRegisterResponse,
  type RemoteKeysApi,
} from "./api";
import { CopyButton } from "../ui/CopyButton";
import { Button, Notice } from "../ui/surface";
import { Field, control } from "../ui/form";
import { PageHeader } from "../ui/page";

type RemoteKeyPanelProps = {
  api?: RemoteKeysApi;
  keys?: Pick<KeysApi, "inventory" | "publicKey">;
  preferredPublicKeyPath?: string | null;
  onPreferredPublicKeyHandled?: () => void;
};

const outcomeLabels: Record<string, MessageKey> = {
  added: "rk.added",
  already_present: "rk.alreadyPresent",
};

const valuesFromLabels: Record<string, MessageKey> = {
  engine: "rk.valuesFromEngine",
  "ssh -G": "rk.valuesFromSshG",
};

export function RemoteKeyPanel({
  api = remoteKeysApi,
  keys = keysApi,
  preferredPublicKeyPath = null,
  onPreferredPublicKeyHandled,
}: RemoteKeyPanelProps) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const [keyPath, setKeyPath] = useState("");
  const [publicKey, setPublicKey] = useState("");
  const [plan, setPlan] = useState<RemoteKeyPlan | null>(null);
  const [plannedInput, setPlannedInput] = useState<RemoteKeyInput | null>(null);
  const [acknowledged, setAcknowledged] = useState(false);
  const [unsupported, setUnsupported] = useState(false);
  const [result, setResult] = useState<RemoteKeyRegisterResponse | null>(null);
  const [planning, setPlanning] = useState(false);
  const [registering, setRegistering] = useState(false);
  const [error, setError] = useState("");
  const [candidates, setCandidates] = useState<KeyItem[]>([]);
  const [chosen, setChosen] = useState("");
  const keyLoadGeneration = useRef(0);
  const planGeneration = useRef(0);
  const preferredHandled = useRef(false);

  const handlePreferredPublicKey = useCallback(() => {
    if (preferredPublicKeyPath === null || preferredHandled.current) return;
    preferredHandled.current = true;
    onPreferredPublicKeyHandled?.();
  }, [preferredPublicKeyPath, onPreferredPublicKeyHandled]);

  useEffect(() => {
    let active = true;
    if (preferredPublicKeyPath !== null) preferredHandled.current = false;
    const preferredRequest = preferredPublicKeyPath === null ? null : ++keyLoadGeneration.current;
    void keys
      .inventory()
      .then(async (inventory) => {
        const publicKeys = inventory.items.filter((item) => item.kind === "public_key");
        if (!active) return;
        setCandidates(publicKeys);
        if (preferredPublicKeyPath === null) return;
        const preferred = publicKeys.find((item) => item.relativePath === preferredPublicKeyPath);
        if (preferred === undefined) return;
        const key = await keys.publicKey(preferred.id);
        if (!active || preferredRequest !== keyLoadGeneration.current) return;
        withdraw();
        setChosen(preferred.id);
        setKeyPath(key.relativePath);
        setPublicKey(key.publicKey.trimEnd());
        setError("");
        handlePreferredPublicKey();
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [keys, onPreferredPublicKeyHandled, preferredPublicKeyPath, handlePreferredPublicKey]);

  function withdraw() {
    planGeneration.current += 1;
    setPlanning(false);
    setPlan(null);
    setPlannedInput(null);
    setAcknowledged(false);
    setUnsupported(false);
    setResult(null);
  }

  function edit(apply: (value: string) => void) {
    return (value: string) => {
      withdraw();
      apply(value);
    };
  }

  function editKey(apply: (value: string) => void) {
    return (value: string) => {
      keyLoadGeneration.current += 1;
      handlePreferredPublicKey();
      setChosen("");
      edit(apply)(value);
    };
  }

  async function choose(keyId: string) {
    const request = ++keyLoadGeneration.current;
    handlePreferredPublicKey();
    withdraw();
    setChosen(keyId);
    if (keyId === "") return;
    try {
      const key = await keys.publicKey(keyId);
      if (request !== keyLoadGeneration.current) return;
      withdraw();
      setKeyPath(key.relativePath);
      setPublicKey(key.publicKey.trimEnd());
      setError("");
    } catch {
      if (request !== keyLoadGeneration.current) return;
      setError(t("rk.publicKeyUnreadable"));
    }
  }

  async function describe() {
    setError("");
    withdraw();
    const request = planGeneration.current;
    const input = { alias, keyPath, publicKey };
    setPlanning(true);
    try {
      const described = await api.plan(input);
      if (request !== planGeneration.current) return;
      setPlan(described);
      setPlannedInput(input);
    } catch (failure) {
      if (request !== planGeneration.current) return;
      setError(describeFailure(failure, t, "rk.planFailed"));
    } finally {
      if (request === planGeneration.current) setPlanning(false);
    }
  }

  async function register() {
    if (plan === null || plannedInput === null) return;
    setError("");
    setRegistering(true);
    try {
      setResult(await api.register({
        ...plannedInput,
        acknowledgeExecutable: acknowledged,
        actionToken: plan.actionToken,
      }));
    } catch (failure) {
      if (failureCode(failure) === "unsupported_remote") setUnsupported(true);
      setError(describeFailure(failure, t, "rk.registerFailed"));
    } finally {
      setRegistering(false);
    }
  }

  const unavoidable = (plan?.executableDirectives ?? []).filter((directive) => !directive.overridable);
  const manual = plan !== null && (!plan.supported || unsupported);
  const busy = planning || registering;
  const blocked = plan === null || plannedInput === null || busy ||
    (unavoidable.length > 0 && !acknowledged);

  return (
    <section aria-label={t("rk.heading")} className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("rk.heading")} description={t("rk.pageDescription")} />

      <p aria-live="polite" className="rounded-lg bg-surface-subtle px-3 py-2 text-sm text-ink-muted">
        {busy ? t("rk.waiting") : t("rk.idle")}
      </p>
      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}


      <section className="sshc-card overflow-hidden rounded-xl bg-card">
        <div className="grid lg:grid-cols-2">
          <div className="border-b border-line p-5 lg:border-b-0 lg:border-r">
            <div className="mb-4 flex items-center gap-3">
              <span aria-hidden="true" className="grid h-7 w-7 place-items-center rounded-full bg-accent text-xs font-semibold text-accent-ink">1</span>
              <h3 className="text-sm font-semibold text-ink">{t("rk.pickFromSsh")}</h3>
            </div>
            <div className="flex flex-col gap-4">
              <Field label={t("rk.pickFromSsh")}>
                <select
                  value={chosen}
                  disabled={busy}
                  onChange={(event) => void choose(event.target.value)}
                  className={control}
                >
                  <option value="">{t("rk.typeInstead")}</option>
                  {candidates.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.fingerprint === "" ? item.relativePath : `${item.relativePath} · ${item.fingerprint}`}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label={t("rk.publicKeyFile")}>
                <input
                  value={keyPath}
                  disabled={busy}
                  onChange={(event) => editKey(setKeyPath)(event.target.value)}
                  className={`${control} font-mono`}
                />
              </Field>
            </div>
          </div>

          <div className="p-5">
            <div className="mb-4 flex items-center gap-3">
              <span aria-hidden="true" className="grid h-7 w-7 place-items-center rounded-full bg-surface text-xs font-semibold text-ink-muted">2</span>
              <h3 className="text-sm font-semibold text-ink">{t("rk.hostAlias")}</h3>
            </div>
            <Field label={t("rk.hostAlias")}>
              <input
                value={alias}
                disabled={busy}
                onChange={(event) => edit(setAlias)(event.target.value)}
                className={`${control} font-mono`}
              />
            </Field>
          </div>
        </div>

        <div className="border-t border-line bg-surface-subtle p-5">
          <Field label={t("rk.publicKeyLine")}>
            <textarea
              value={publicKey}
              disabled={busy}
              onChange={(event) => editKey(setPublicKey)(event.target.value)}
              rows={3}
              className={`${control} font-mono text-xs leading-5`}
            />
          </Field>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-4">
          <div className="flex items-center gap-2 text-xs text-ink-muted">
            <span aria-hidden="true" className={`h-2 w-2 rounded-full ${plan === null ? "bg-ink-faint" : "bg-live"}`} />
            {plan === null ? t("rk.showWhatWouldHappen") : t("rk.confirmHeading")}
          </div>
          <div className="flex gap-2">
            <Button disabled={busy} onClick={() => void describe()}>{t("rk.showWhatWouldHappen")}</Button>
            {manual ? null : (
              <Button kind="primary" disabled={blocked} onClick={() => void register()}>
                {t("rk.register")}
              </Button>
            )}
          </div>
        </div>
      </section>

      {plan ? (
        <section aria-labelledby="remote-key-plan-heading" className="sshc-card overflow-hidden rounded-xl bg-card text-sm">
          <div className="flex items-center gap-3 border-b border-line bg-surface-subtle px-5 py-4">
            <span aria-hidden="true" className="grid h-7 w-7 place-items-center rounded-full bg-live text-xs font-semibold text-accent-ink">3</span>
            <h3 id="remote-key-plan-heading" className="font-semibold">{t("rk.confirmHeading")}</h3>
          </div>

          <div className="grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
            <dl className="border-b border-line lg:border-b-0 lg:border-r">
              {[
                [t("rk.alias"), plan.alias],
                [t("rk.effectiveUser"), plan.user === "" ? t("rk.noUser") : plan.user],
                [t("rk.destination"), `${plan.hostname}:${plan.port}`],
                [
                  t("rk.valuesCameFrom"),
                  plan.valuesFrom in valuesFromLabels ? t(valuesFromLabels[plan.valuesFrom]!) : plan.valuesFrom,
                ],
                [t("rk.keyFile"), plan.keyPath],
                [t("rk.fingerprint"), plan.fingerprint],
              ].map(([caption, value]) => (
                <div
                  key={caption}
                  className="grid gap-1 border-t border-hairline px-5 py-3 first:border-t-0 sm:grid-cols-[9rem_minmax(0,1fr)]"
                >
                  <dt className="text-xs font-medium text-ink-muted">{caption}</dt>
                  <dd className="min-w-0 break-all font-mono text-xs">{value}</dd>
                </div>
              ))}
            </dl>

            <div className="p-5">
          <p>
            {t("rk.appendTo", {
              remotePath: plan.remotePath,
              account: plan.user === "" ? t("rk.theRemoteAccount") : t("rk.usersAccount", { user: plan.user }),
              hostname: plan.hostname,
            })}
          </p>
          <pre
            aria-label={t("rk.keyLineLabel")}
            className="mt-3 overflow-x-auto rounded-lg border border-line bg-canvas p-3 font-mono text-xs"
          >
            {plan.keyLine}
          </pre>
          <div className="mt-2">
            <CopyButton value={plan.keyLine} label="copy.keyLine" />
          </div>
          <p className="mt-5 text-ink-muted">{t("rk.remoteRuns")}</p>
          <pre aria-label={t("rk.remoteCommandLabel")} className="mt-2 overflow-x-auto rounded-lg border border-line bg-canvas p-3 font-mono text-xs">
            {plan.routine}
          </pre>
          <div className="mt-1">
            <CopyButton value={plan.routine} label="copy.remoteCommand" />
          </div>

          {unavoidable.length > 0 ? (
            <div className="mt-4 rounded-lg border border-notice-line bg-notice p-3">
              <h4 className="font-medium text-notice-ink">{t("rk.connectingRuns")}</h4>
              <ul className="mt-1 flex flex-col gap-1">
                {unavoidable.map((directive) => (
                  <li key={`${directive.path}:${directive.line}:${directive.keyword}`}>
                    <span className="text-ink-muted">
                      {directive.keyword} at {directive.path}:{directive.line}
                    </span>
                    <pre className="whitespace-pre-wrap break-all text-ink">{directive.command}</pre>
                  </li>
                ))}
              </ul>
              <label className="mt-2 flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={acknowledged}
                  onChange={(event) => setAcknowledged(event.target.checked)}
                />
                <span>{t("rk.acknowledgeRuns")}</span>
              </label>
            </div>
          ) : null}

          {manual ? (
            <div className="mt-4 rounded-lg bg-surface-subtle p-3">
              <h4 className="font-medium text-notice-ink">
                {t("rk.manualHeading")}
              </h4>
              <ol className="mt-1 list-decimal pl-5">
                {plan.manual.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
            </div>
          ) : null}
            </div>
          </div>
        </section>
      ) : null}

      {result ? (
        <div className="sshc-card rounded-xl bg-card p-4 text-sm">
          <h3 className="font-semibold text-live">{t("rk.result")}</h3>
          <p>{result.outcome in outcomeLabels ? t(outcomeLabels[result.outcome]!) : result.outcome}</p>
          {result.stderr ? (
            <pre className="whitespace-pre-wrap break-all text-ink-muted">{result.stderr}</pre>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function describeFailure(failure: unknown, t: Translate, fallback: MessageKey): string {
  const code = failureCode(failure);
  return code === "" ? t(fallback) : t("rk.withCode", { message: t(fallback), code });
}

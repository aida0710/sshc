import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import { Button, Card, Notice } from "../ui/surface";
import { CheckboxField, Field, control } from "../ui/form";
import { PageHeader } from "../ui/page";

type RemoteKeyPanelProps = {
  api?: RemoteKeysApi;
  keys?: Pick<KeysApi, "inventory" | "publicKey">;
  hosts?: string[];
  preferredPublicKeyPath?: string | null;
  onPreferredPublicKeyHandled?: () => void;
};

type PlannedTarget = {
  input: RemoteKeyInput;
  plan: RemoteKeyPlan;
};

type RegistrationOutcome = {
  alias: string;
  response?: RemoteKeyRegisterResponse;
  error?: string;
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
  hosts = [],
  preferredPublicKeyPath = null,
  onPreferredPublicKeyHandled,
}: RemoteKeyPanelProps) {
  const t = useTranslate();
  const [hostQuery, setHostQuery] = useState("");
  const [selectedAliases, setSelectedAliases] = useState<string[]>([]);
  const [keyPath, setKeyPath] = useState("");
  const [publicKey, setPublicKey] = useState("");
  const [plannedTargets, setPlannedTargets] = useState<PlannedTarget[]>([]);
  const [acknowledged, setAcknowledged] = useState(false);
  const [unsupportedAliases, setUnsupportedAliases] = useState<ReadonlySet<string>>(new Set());
  const [results, setResults] = useState<RegistrationOutcome[]>([]);
  const [planning, setPlanning] = useState(false);
  const [registering, setRegistering] = useState(false);
  const [error, setError] = useState("");
  const [candidates, setCandidates] = useState<KeyItem[]>([]);
  const [chosen, setChosen] = useState("");
  const keyLoadGeneration = useRef(0);
  const planGeneration = useRef(0);
  const preferredHandled = useRef(false);

  const hostOptions = useMemo(
    () => [...new Set(hosts.filter((host) => host !== ""))]
      .sort((left, right) => left.localeCompare(right, undefined, { sensitivity: "base", numeric: true })),
    [hosts],
  );
  const matchingHosts = useMemo(() => {
    const query = hostQuery.trim().toLocaleLowerCase();
    if (query === "") return hostOptions;
    return hostOptions.filter((host) => host.toLocaleLowerCase().includes(query));
  }, [hostOptions, hostQuery]);

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
    setPlannedTargets([]);
    setAcknowledged(false);
    setUnsupportedAliases(new Set());
    setResults([]);
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

  function toggleAlias(alias: string) {
    withdraw();
    setSelectedAliases((current) => current.includes(alias)
      ? current.filter((item) => item !== alias)
      : [...current, alias]);
  }

  function selectMatchingHosts() {
    if (matchingHosts.length === 0) return;
    withdraw();
    setSelectedAliases((current) => [...new Set([...current, ...matchingHosts])]);
  }

  function clearSelectedHosts() {
    withdraw();
    setSelectedAliases([]);
    setHostQuery("");
  }

  async function describe() {
    setError("");
    withdraw();
    const request = planGeneration.current;
    const typedAlias = hostQuery.trim();
    const aliases = selectedAliases.length > 0
      ? selectedAliases
      : typedAlias === "" ? [] : [typedAlias];
    if (aliases.length === 0) {
      setError(t("rk.chooseHost"));
      return;
    }
    setPlanning(true);
    const inputs = aliases.map((alias) => ({ alias, keyPath, publicKey }));
    const settled = await settleWithLimit(inputs, 4, (input) => api.plan(input));
    if (request !== planGeneration.current) return;
    const planned = settled.flatMap((outcome, index) => outcome.status === "fulfilled"
      ? [{ input: inputs[index]!, plan: outcome.value }]
      : []);
    const failures = settled.flatMap((outcome, index) => outcome.status === "rejected"
      ? [`${inputs[index]!.alias}: ${describeFailure(outcome.reason, t, "rk.planFailed")}`]
      : []);
    setPlannedTargets(planned);
    if (failures.length > 0) setError(failures.join("\n"));
    setPlanning(false);
  }

  async function register() {
    const registerable = plannedTargets.filter(({ plan }) => plan.supported && !unsupportedAliases.has(plan.alias));
    if (registerable.length === 0) return;
    setError("");
    setRegistering(true);
    const settled = await settleWithLimit(registerable, 4, ({ input, plan }) => api.register({
        ...input,
        acknowledgeExecutable: acknowledged,
        actionToken: plan.actionToken,
      }));
    const newlyUnsupported = new Set(unsupportedAliases);
    const completed = settled.map<RegistrationOutcome>((outcome, index) => {
      const alias = registerable[index]!.plan.alias;
      if (outcome.status === "fulfilled") return { alias, response: outcome.value };
      if (failureCode(outcome.reason) === "unsupported_remote") newlyUnsupported.add(alias);
      return { alias, error: describeFailure(outcome.reason, t, "rk.registerFailed") };
    });
    setUnsupportedAliases(newlyUnsupported);
    setResults(completed);
    const failed = completed.filter((outcome) => outcome.error !== undefined);
    if (failed.length > 0) {
      const firstError = failed[0]?.error;
      setError(completed.length === 1 && firstError !== undefined
        ? firstError
        : t("rk.someRegistrationsFailed"));
    }
    setRegistering(false);
  }

  const unavoidable = plannedTargets.flatMap(({ plan }) =>
    plan.executableDirectives
      .filter((directive) => !directive.overridable)
      .map((directive) => ({ alias: plan.alias, directive })),
  );
  const registerable = plannedTargets.filter(({ plan }) => plan.supported && !unsupportedAliases.has(plan.alias));
  const busy = planning || registering;
  const blocked = registerable.length === 0 || busy ||
    (unavoidable.length > 0 && !acknowledged);

  return (
    <section aria-label={t("rk.heading")} className="mx-auto flex w-full max-w-5xl flex-col gap-6 [&_button]:min-h-10 sm:[&_button]:min-h-0">
      <PageHeader title={t("rk.heading")} description={t("rk.pageDescription")} />

      <p aria-live="polite" className="rounded-lg bg-surface-subtle px-3 py-2 text-sm text-ink-muted">
        {busy ? t("rk.waiting") : t("rk.idle")}
      </p>
      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}


      <Card as="section" radius="md">
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
            <div className="flex flex-col gap-3">
              <Field label={t("rk.hostSearch")}>
                <input
                  type="search"
                  aria-label={t("rk.hostAlias")}
                  value={hostQuery}
                  disabled={busy}
                  placeholder={t("rk.hostSearchPlaceholder")}
                  onChange={(event) => {
                    if (selectedAliases.length === 0) withdraw();
                    setHostQuery(event.target.value);
                  }}
                  className={`${control} font-mono`}
                />
              </Field>
              {hostOptions.length === 0 ? (
                <p className="text-xs text-ink-muted">{t("rk.hostTypeHint")}</p>
              ) : (
                <div className="overflow-hidden rounded-md border border-line">
                  <div className="flex items-center justify-between gap-2 border-b border-line bg-surface-subtle px-3 py-2">
                    <span className="text-xs text-ink-muted">
                      {t("rk.hostsSelected", { count: selectedAliases.length })}
                    </span>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        disabled={busy || matchingHosts.length === 0}
                        onClick={selectMatchingHosts}
                        className="text-xs text-accent hover:underline disabled:text-ink-faint disabled:no-underline"
                      >
                        {t("rk.selectMatches")}
                      </button>
                      <button
                        type="button"
                        disabled={busy || selectedAliases.length === 0}
                        onClick={clearSelectedHosts}
                        className="text-xs text-ink-muted hover:text-ink disabled:text-ink-faint"
                      >
                        {t("rk.clearSelection")}
                      </button>
                    </div>
                  </div>
                  <div role="group" aria-label={t("rk.hostChoices")} className="max-h-44 overflow-y-auto p-1.5">
                    {matchingHosts.length === 0 ? (
                      <p className="px-2 py-4 text-center text-xs text-ink-muted">{t("rk.noHostMatches")}</p>
                    ) : matchingHosts.map((host) => (
                      <label key={host} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-select-fill">
                        <input
                          type="checkbox"
                          checked={selectedAliases.includes(host)}
                          disabled={busy}
                          onChange={() => toggleAlias(host)}
                        />
                        <span className="min-w-0 truncate font-mono">{host}</span>
                      </label>
                    ))}
                  </div>
                </div>
              )}
            </div>
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
            <span aria-hidden="true" className={`h-2 w-2 rounded-full ${plannedTargets.length === 0 ? "bg-ink-faint" : "bg-live"}`} />
            {plannedTargets.length === 0
              ? t("rk.showWhatWouldHappen")
              : t("rk.plannedHosts", { count: plannedTargets.length })}
          </div>
          <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
            <Button className="w-full sm:w-auto" disabled={busy} onClick={() => void describe()}>{t("rk.showWhatWouldHappen")}</Button>
            {plannedTargets.length > 0 && registerable.length === 0 ? null : (
              <Button className="w-full sm:w-auto" kind="primary" disabled={blocked} onClick={() => void register()}>
                {registerable.length > 1 ? t("rk.registerMany", { count: registerable.length }) : t("rk.register")}
              </Button>
            )}
          </div>
        </div>
      </Card>

      {plannedTargets.length > 0 ? (
        <Card as="section" aria-labelledby="remote-key-plan-heading" radius="md" className="text-sm">
          <div className="flex items-center gap-3 border-b border-line bg-surface-subtle px-5 py-4">
            <span aria-hidden="true" className="grid h-7 w-7 place-items-center rounded-full bg-live text-xs font-semibold text-accent-ink">3</span>
            <h3 id="remote-key-plan-heading" className="font-semibold">
              {plannedTargets.length === 1
                ? t("rk.confirmHeading")
                : t("rk.confirmManyHeading", { count: plannedTargets.length })}
            </h3>
          </div>
          <div className="divide-y divide-line">
            {plannedTargets.map(({ plan }) => (
              <RemoteKeyPlanCard
                key={plan.alias}
                plan={plan}
                manual={!plan.supported || unsupportedAliases.has(plan.alias)}
              />
            ))}
          </div>
          {unavoidable.length > 0 ? (
            <div className="border-t border-notice-line bg-notice p-4">
              <h4 className="font-medium text-notice-ink">{t("rk.connectingRuns")}</h4>
              <ul className="mt-2 flex flex-col gap-2">
                {unavoidable.map(({ alias, directive }) => (
                  <li key={`${alias}:${directive.path}:${directive.line}:${directive.keyword}`}>
                    <span className="text-ink-muted">
                      {alias} · {directive.keyword} at {directive.path}:{directive.line}
                    </span>
                    <pre className="whitespace-pre-wrap break-all text-ink">{directive.command}</pre>
                  </li>
                ))}
              </ul>
              <CheckboxField className="mt-2" label={t("rk.acknowledgeRuns")} checked={acknowledged} onChange={setAcknowledged} tone="notice" />
            </div>
          ) : null}
        </Card>
      ) : null}

      {results.length > 0 ? (
        <Card as="section" radius="md" className="text-sm">
          <h3 className="px-4 pt-4 font-semibold text-live">{t("rk.result")}</h3>
          <ul className="mt-2 divide-y divide-line">
            {results.map((result) => (
              <li key={result.alias} className="grid gap-1 px-4 py-3 sm:grid-cols-[12rem_minmax(0,1fr)]">
                <span className="font-mono font-medium">{result.alias}</span>
                <div>
                  {result.response === undefined ? (
                    <p className="text-danger">{result.error}</p>
                  ) : (
                    <>
                      <p>{result.response.outcome in outcomeLabels
                        ? t(outcomeLabels[result.response.outcome]!)
                        : result.response.outcome}</p>
                      {result.response.stderr ? (
                        <pre className="whitespace-pre-wrap break-all text-ink-muted">{result.response.stderr}</pre>
                      ) : null}
                    </>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
    </section>
  );
}

function RemoteKeyPlanCard({ plan, manual }: { plan: RemoteKeyPlan; manual: boolean }) {
  const t = useTranslate();
  return (
    <article aria-label={t("rk.planFor", { alias: plan.alias })}>
      <h4 className="border-b border-hairline px-5 py-3 font-mono font-semibold text-ink">{plan.alias}</h4>
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
            <div key={caption} className="grid gap-1 border-t border-hairline px-5 py-3 first:border-t-0 sm:grid-cols-[9rem_minmax(0,1fr)]">
              <dt className="text-xs font-medium text-ink-muted">{caption}</dt>
              <dd className="min-w-0 break-all font-mono text-xs">{value}</dd>
            </div>
          ))}
        </dl>
        <div className="p-5">
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
            <div className="mt-4 rounded-lg bg-surface-subtle p-3">
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

function describeFailure(failure: unknown, t: Translate, fallback: MessageKey): string {
  const code = failureCode(failure);
  return code === "" ? t(fallback) : t("rk.withCode", { message: t(fallback), code });
}

async function settleWithLimit<Input, Output>(
  inputs: Input[],
  limit: number,
  operation: (input: Input) => Promise<Output>,
): Promise<PromiseSettledResult<Output>[]> {
  const results = new Array<PromiseSettledResult<Output>>(inputs.length);
  let next = 0;
  async function worker() {
    while (next < inputs.length) {
      const index = next++;
      try {
        results[index] = { status: "fulfilled", value: await operation(inputs[index]!) };
      } catch (reason) {
        results[index] = { status: "rejected", reason };
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, inputs.length) }, () => worker()));
  return results;
}

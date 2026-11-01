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
import { ActionArea } from "../ui/ActionArea";
import { FieldRow, FieldRowNote } from "../ui/FieldRow";
import { Button, Notice } from "../ui/surface";
import { CheckboxField, control } from "../ui/form";
import { PageHeader, PageSection } from "../ui/page";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { RemoteKeyHostPicker } from "./RemoteKeyHostPicker";
import { RemoteKeyPlanCard } from "./RemoteKeyPlanCard";
import { unavoidableDirectives } from "../diagnostics/executableDirectives";
import { literalTextEntry } from "../ui/softKeyboard";

// 公開鍵をリモートホストの authorized_keys へ登録する画面。「1. 公開鍵」で鍵を、「2. 接続先」で
// ホストを選び、［登録内容を確認］で sshcエンジンが設定から求めた登録内容（接続しない）を見せて
// から、［鍵を登録］で初めてホストへ接続する。

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
  // 読めなかったのか公開鍵が無いのかを見分けられるように、失敗は別に持つ。
  const [candidatesFailed, setCandidatesFailed] = useState(false);
  const [chosen, setChosen] = useState("");
  const keyLoadGeneration = useRequestGeneration();
  const planGeneration = useRequestGeneration();
  const preferredHandled = useRef(false);
  // keyEntered は、公開鍵の欄が一度でも埋まった（選んだ・入力した・渡された）ことを表す。
  const keyEntered = useRef(false);

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

  const withdraw = useCallback(() => {
    planGeneration.retire();
    setPlanning(false);
    setPlannedTargets([]);
    setAcknowledged(false);
    setUnsupportedAliases(new Set());
    setResults([]);
  }, [planGeneration]);

  // 開いたときは ~/.ssh の最初の公開鍵（Keys から渡された鍵があればその鍵）を選んでおく。
  // 渡された鍵は、それより前に始めた読み込みに勝つ。最初の公開鍵は、読み終える前に利用者が
  // 鍵を選んだり入力したりしたら入れず、一度でも鍵の欄が埋まったあとは選び直さない。
  useEffect(() => {
    let active = true;
    if (preferredPublicKeyPath !== null) preferredHandled.current = false;
    const isCurrent = preferredPublicKeyPath === null ? keyLoadGeneration.observe() : keyLoadGeneration.begin();
    void keys
      .inventory()
      .then(async (inventory) => {
        const publicKeys = inventory.items.filter((item) => item.kind === "public_key");
        if (!active) return;
        setCandidates(publicKeys);
        setCandidatesFailed(false);
        const preferred = preferredPublicKeyPath === null
          ? undefined
          : publicKeys.find((item) => item.relativePath === preferredPublicKeyPath);
        const initial = preferred ?? (keyEntered.current ? undefined : publicKeys[0]);
        if (initial === undefined) return;
        const key = await keys.publicKey(initial.id);
        if (!active || !isCurrent()) return;
        withdraw();
        keyEntered.current = true;
        setChosen(initial.id);
        setKeyPath(key.relativePath);
        setPublicKey(key.publicKey.trimEnd());
        setError("");
        if (preferred !== undefined) handlePreferredPublicKey();
      })
      .catch(() => {
        if (active) setCandidatesFailed(true);
      });
    return () => {
      active = false;
    };
  }, [keys, keyLoadGeneration, preferredPublicKeyPath, handlePreferredPublicKey, withdraw]);

  function edit(apply: (value: string) => void) {
    return (value: string) => {
      withdraw();
      apply(value);
    };
  }

  function editKey(apply: (value: string) => void) {
    return (value: string) => {
      keyLoadGeneration.retire();
      keyEntered.current = true;
      handlePreferredPublicKey();
      setChosen("");
      edit(apply)(value);
    };
  }

  async function choose(keyId: string) {
    const isCurrent = keyLoadGeneration.begin();
    keyEntered.current = true;
    handlePreferredPublicKey();
    withdraw();
    setChosen(keyId);
    if (keyId === "") return;
    try {
      const key = await keys.publicKey(keyId);
      if (!isCurrent()) return;
      withdraw();
      setKeyPath(key.relativePath);
      setPublicKey(key.publicKey.trimEnd());
      setError("");
    } catch {
      if (!isCurrent()) return;
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
    const isCurrent = planGeneration.observe();
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
    if (!isCurrent()) return;
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
    unavoidableDirectives(plan.executableDirectives).map((directive) => ({ alias: plan.alias, directive })),
  );
  const registerable = plannedTargets.filter(({ plan }) => plan.supported && !unsupportedAliases.has(plan.alias));
  const busy = planning || registering;
  const blocked = registerable.length === 0 || busy ||
    (unavoidable.length > 0 && !acknowledged);

  return (
    <section aria-label={t("rk.heading")} className="flex flex-col gap-[22px] [&_button]:min-h-10 sm:[&_button]:min-h-0">
      <PageHeader title={t("rk.heading")} description={t("rk.description")} />

      {error ? (
        <Notice tone="danger">{error}</Notice>
      ) : null}

      <PageSection heading={t("rk.publicKeyStep")}>
        <div>
          <FieldRow label={t("rk.pickFromSsh")}>
            <select
              value={chosen}
              disabled={busy}
              onChange={(event) => void choose(event.target.value)}
              className={control}
            >
              {candidates.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.comment === "" ? item.relativePath : t("rk.keyChoice", { path: item.relativePath, comment: item.comment })}
                </option>
              ))}
              <option value="">{t("rk.typeInstead")}</option>
            </select>
          </FieldRow>
          {candidatesFailed ? <FieldRowNote><Notice compact>{t("rk.candidatesLoadFailed")}</Notice></FieldRowNote> : null}
          {/* ~/.ssh から選んだ鍵はファイルが決まっているので、ファイルの欄は手で入力するときだけ出す。 */}
          {chosen === "" ? (
            <FieldRow label={t("rk.publicKeyFile")}>
              <input {...literalTextEntry}
                value={keyPath}
                disabled={busy}
                onChange={(event) => editKey(setKeyPath)(event.target.value)}
                className={`${control} font-mono`}
              />
            </FieldRow>
          ) : null}
          <FieldRow label={t("rk.publicKeyLine")}>
            {/* 文字の大きさは、control の大きさより後に効くよう sm: で付ける（狭い画面は index.css が16pxにする）。 */}
            <textarea
              value={publicKey}
              disabled={busy}
              onChange={(event) => editKey(setPublicKey)(event.target.value)}
              rows={2}
              spellCheck={false}
              className={`${control} break-all font-mono leading-5 text-ink-muted max-sm:h-24 sm:text-[12.5px]`}
            />
          </FieldRow>
        </div>
      </PageSection>

      <PageSection heading={t("rk.hostStep")}>
        <RemoteKeyHostPicker
          query={hostQuery}
          hostOptions={hostOptions}
          matchingHosts={matchingHosts}
          selectedAliases={selectedAliases}
          busy={busy}
          onQueryChange={(query) => {
            // 何も選んでいなければ、入力した alias が接続先なので、確認した内容は取り下げる。
            if (selectedAliases.length === 0) withdraw();
            setHostQuery(query);
          }}
          onToggle={toggleAlias}
          onSelectMatches={selectMatchingHosts}
          onClear={clearSelectedHosts}
        />
        <ActionArea
          status={plannedTargets.length === 0 ? undefined : (
            <p className="text-xs text-ink-muted">{t("rk.plannedHosts", { count: plannedTargets.length })}</p>
          )}
        >
          <div className="flex flex-wrap justify-end gap-2">
            <Button busy={planning} disabled={busy} onClick={() => void describe()}>{t("rk.showWhatWouldHappen")}</Button>
            {plannedTargets.length > 0 && registerable.length === 0 ? null : (
              <Button kind="primary" busy={registering} disabled={blocked} onClick={() => void register()}>
                {registerable.length > 1 ? t("rk.registerMany", { count: registerable.length }) : t("rk.register")}
              </Button>
            )}
          </div>
        </ActionArea>
      </PageSection>

      {plannedTargets.length > 0 ? (
        <PageSection
          heading={plannedTargets.length === 1
            ? t("rk.confirmHeading")
            : t("rk.confirmManyHeading", { count: plannedTargets.length })}
        >
          <div className="divide-y divide-line text-sm">
            {plannedTargets.map(({ plan }) => (
              <RemoteKeyPlanCard
                key={plan.alias}
                plan={plan}
                manual={!plan.supported || unsupportedAliases.has(plan.alias)}
              />
            ))}
          </div>
          {unavoidable.length > 0 ? (
            <div className="mt-3 rounded-[10px] bg-notice p-4 text-sm">
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
        </PageSection>
      ) : null}

      {results.length > 0 ? (
        <PageSection heading={t("rk.result")}>
          <ul className="divide-y divide-line text-sm">
            {results.map((result) => (
              <li key={result.alias} className="grid gap-1 py-3 first:pt-0 sm:grid-cols-[12rem_minmax(0,1fr)]">
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
        </PageSection>
      ) : null}
    </section>
  );
}

// failureMessages は、利用者が直すものがほかの失敗と違う code に、専用の文を割り当てる。
const failureMessages: Readonly<Record<string, MessageKey>> = {
  config_not_confirmable: "rk.configNotConfirmable",
};

function describeFailure(failure: unknown, t: Translate, fallback: MessageKey): string {
  const code = failureCode(failure);
  if (code === "") return t(fallback);
  return t("rk.withCode", { message: t(failureMessages[code] ?? fallback), code });
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

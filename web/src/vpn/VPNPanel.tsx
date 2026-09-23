import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { vpnApi, type VPNApi, type VPNOverview, type VPNProfile, type VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { hintText } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { usePolling } from "../ui/usePolling";
import { vpnFailureReasonMessage } from "./vpnFailureReasons";
import { vpnFieldErrorOf, type VPNFieldError } from "./vpnFieldErrors";
import { VPNLogsDialog } from "./VPNLogsDialog";
import { VPNProfileCard } from "./VPNProfileCard";
import { VPNProfileForm, type VPNProfileSaveResult } from "./VPNProfileForm";
import { routeProgressIntervalMs } from "./vpnPhases";
import { vpnRefusals } from "./vpnRefusals";

// 接続ごとのVPN経路の画面。トンネルはengineが持つコンテナの中にあり、ここでは
// プロファイルの定義と、いまの状態と、どの接続がそれを通るかを扱う。秘密は保存の
// ときに送るだけで、engineは決して返さない。

type VPNPanelProps = {
  api?: VPNApi;
  // aliases は、紐付けの相手に選べる接続である。
  aliases?: string[];
};

export function VPNPanel({ api = vpnApi, aliases = [] }: VPNPanelProps) {
  const t = useTranslate();
  const operation = useAsyncOperation();
  const [overview, setOverview] = useState<VPNOverview | null>(null);
  const [pendingRemoval, setPendingRemoval] = useState("");
  const [pendingRename, setPendingRename] = useState("");
  const [shownLogs, setShownLogs] = useState("");
  // startingProfile は、いま経路を用意させているプロファイルである。
  const [startingProfile, setStartingProfile] = useState("");
  // failedProfile は、直前に経路を用意できなかったプロファイルである。失敗の文の
  // 横からそのログを開けるようにする。
  const [failedProfile, setFailedProfile] = useState("");

  // 一覧を書き換えた応答の世代。操作を始めるときと終えるときに進め、読み始めたあとに
  // 世代が変わった応答は捨てる。遅れて届いたポーリングの応答が、操作の運んだ新しい
  // 一覧を古い一覧で上書きしないためである。
  const generation = useRef(0);

  const describe = useCallback(
    (error: unknown) => {
      const failure = vpnFailureReasonMessage(error);
      if (failure !== null) return t("vpn.sessionFailed", { reason: t(failure) });
      const code = failureCode(error);
      const key = code === undefined ? undefined : vpnRefusals[code];
      return key === undefined ? t("vpn.failed") : t(key);
    },
    [t],
  );

  const run = operation.run;
  // act は、一覧を返す操作を走らせる。応答の一覧は、その間に別の操作が始まって
  // いなければ採る。失敗の言い方は describeFailure で変えられる。成功したかどうかを返す。
  const act = useCallback(
    async (work: () => Promise<VPNOverview>, describeFailure: (error: unknown) => string = describe) => {
      generation.current += 1;
      const started = generation.current;
      setFailedProfile("");
      const succeeded = await run(work, {
        apply: (next) => {
          if (generation.current === started) setOverview(next);
        },
        describe: describeFailure,
      });
      if (generation.current === started) generation.current += 1;
      return succeeded;
    },
    [describe, run],
  );

  useEffect(() => {
    // 開いたときに一度だけ読む。以降は、操作の応答が最新の一覧を運ぶ。
    void act(() => api.vpnOverview());
  }, [api, act]);

  const startProfile = useCallback(
    async (name: string) => {
      setStartingProfile(name);
      await act(
        () => api.startVPNSession(name),
        (error) => {
          if (vpnFailureReasonMessage(error) !== null) setFailedProfile(name);
          return describe(error);
        },
      );
      setStartingProfile("");
    },
    [act, api, describe],
  );

  const createProfile = useCallback(
    async (profile: VPNProfile, secrets: VPNSecrets): Promise<VPNProfileSaveResult> => {
      let fieldError: VPNFieldError | null = null;
      const saved = await act(
        () => api.createVPNProfile(profile, secrets),
        (error) => {
          // 項目の誤りは、その項目の横に理由を出す。ここでは横を見るよう促すだけにする。
          fieldError = vpnFieldErrorOf(error);
          return fieldError === null ? describe(error) : t("vpn.fieldRefused");
        },
      );
      return saved ? { saved: true } : { saved: false, fieldError };
    },
    [act, api, describe, t],
  );

  // 経路が立つまでは分単位になることがある。待っているあいだだけ状態を読み直し、
  // どこまで進んだかを見せる。ほかの操作の最中は読み直さない。その応答が一覧を
  // 運んでくるからである。経路を用意させている最中だけは、その応答が経路が立つまで
  // 返らないので、段階を見せるために読み直し続ける。
  const preparingRoute = overview?.profiles.some((status) => (status.phase ?? "") !== "") ?? false;
  const pollProgress = startingProfile !== "" || (!operation.busy && preparingRoute);
  usePolling(
    () => {
      const started = generation.current;
      return api
        .vpnOverview()
        .then((next) => {
          if (generation.current === started) setOverview(next);
        })
        .catch(() => undefined);
    },
    { intervalMs: routeProgressIntervalMs, enabled: pollProgress },
  );

  if (overview === null) {
    return (
      <PanelState
        tone={operation.error === "" ? "loading" : "failed"}
        title={operation.error === "" ? t("vpn.loading") : operation.error}
      />
    );
  }

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("vpn.heading")} description={t("vpn.description")} />

      {overview.available ? null : (
        <Notice>{t("vpn.unavailable", { detail: overview.detail ?? "" })}</Notice>
      )}
      {operation.error === "" ? null : (
        <Notice tone="danger">
          <span className="grow">{operation.error}</span>
          {failedProfile === "" ? null : (
            <Button className="shrink-0" onClick={() => setShownLogs(failedProfile)}>
              {t("vpn.failureShowLogs")}
            </Button>
          )}
        </Notice>
      )}

      {overview.profiles.length === 0 ? (
        <p className={hintText}>{t("vpn.empty")}</p>
      ) : (
        <ul className="flex flex-col gap-3">
          {overview.profiles.map((status) => {
            const name = status.profile.name;
            return (
              <li key={name}>
                <VPNProfileCard
                  status={status}
                  aliases={aliases}
                  busy={operation.busy}
                  available={overview.available}
                  actions={{
                    onStart: () => void startProfile(name),
                    onStop: () => void act(() => api.stopVPNSession(name)),
                    onShowLogs: () => setShownLogs(name),
                    onRename: () => setPendingRename(name),
                    onRemove: () => setPendingRemoval(name),
                    onBind: (alias) => void act(() => api.setConnectionVPN(alias, name)),
                    onUnbind: (alias) => void act(() => api.setConnectionVPN(alias, "")),
                  }}
                />
              </li>
            );
          })}
        </ul>
      )}

      <VPNProfileForm
        busy={operation.busy}
        onSave={createProfile}
      />

      {pendingRemoval === "" ? null : (
        <ConfirmDialog
          id="vpn-remove"
          heading={t("vpn.removeTitle", { name: pendingRemoval })}
          body={t("vpn.removeBody")}
          confirmLabel={t("vpn.remove")}
          cancelLabel={t("vpn.cancel")}
          onCancel={() => setPendingRemoval("")}
          onConfirm={() => {
            const name = pendingRemoval;
            setPendingRemoval("");
            void act(() => api.removeVPNProfile(name));
          }}
        />
      )}

      {pendingRename === "" ? null : (
        <InputDialog
          id="vpn-rename"
          heading={t("vpn.renameTitle", { name: pendingRename })}
          description={t("vpn.renameHint")}
          label={t("vpn.renameLabel")}
          initialValue={pendingRename}
          submitLabel={t("vpn.renameAction")}
          cancelLabel={t("vpn.cancel")}
          validate={(value) => (value === "" ? t("vpn.renameEmpty") : "")}
          onCancel={() => setPendingRename("")}
          onSubmit={(value) => {
            const from = pendingRename;
            setPendingRename("");
            void act(() => api.renameVPNProfile(from, value));
          }}
        />
      )}

      {shownLogs === "" ? null : (
        <VPNLogsDialog
          name={shownLogs}
          api={api}
          describe={describe}
          onClose={() => setShownLogs("")}
        />
      )}
    </section>
  );
}

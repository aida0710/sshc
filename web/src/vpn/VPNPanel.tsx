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
import { describeVPNFieldError, vpnFieldErrorOf, type VPNFieldError } from "./vpnFieldErrors";
import { VPNLogsDialog } from "./VPNLogsDialog";
import { VPNProfileCard } from "./VPNProfileCard";
import { VPNProfileForm, type VPNProfileSaveResult } from "./VPNProfileForm";
import { routeProgressIntervalMs } from "./vpnPhases";
import { describeVPNProblem } from "./vpnProblemMessage";
import { vpnProfileNameError } from "./vpnProfileRules";
import { VPNUnavailableNotice } from "./VPNUnavailableNotice";

// 接続ごとのVPN経路の画面。トンネルはengineが持つコンテナの中にあり、ここでは
// プロファイルの定義と、いまの状態と、どの接続がそれを使うかを扱う。接続へ
// プロファイルを付けるのは Connections で行う。シークレットは保存のときに送るだけで、
// engineは決して返さない。

type VPNPanelProps = {
  api?: VPNApi;
};

export function VPNPanel({ api = vpnApi }: VPNPanelProps) {
  const t = useTranslate();
  const operation = useAsyncOperation();
  const [overview, setOverview] = useState<VPNOverview | null>(null);
  const [pendingRemoval, setPendingRemoval] = useState("");
  const [pendingRename, setPendingRename] = useState("");
  // editingProfile は、いま編集のフォームを開いているプロファイルである。
  const [editingProfile, setEditingProfile] = useState("");
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

  const describe = useCallback((error: unknown) => describeVPNProblem(t, error) ?? t("vpn.failed"), [t]);

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
          if (failureCode(error) === "vpn_session_failed") setFailedProfile(name);
          return describe(error);
        },
      );
      setStartingProfile("");
    },
    [act, api, describe],
  );

  // saveProfile は、プロファイルを保存する操作を走らせる。項目の誤りは、フォームの
  // その項目の横に理由を出す。ここでは横を見るよう促すだけにする。
  const saveProfile = useCallback(
    async (work: () => Promise<VPNOverview>): Promise<VPNProfileSaveResult> => {
      let fieldError: VPNFieldError | null = null;
      const saved = await act(work, (error) => {
        fieldError = vpnFieldErrorOf(error);
        return fieldError === null ? describe(error) : t("vpn.fieldRefused");
      });
      return saved ? { saved: true } : { saved: false, fieldError };
    },
    [act, describe, t],
  );

  const createProfile = useCallback(
    (profile: VPNProfile, secrets: VPNSecrets) => saveProfile(() => api.createVPNProfile(profile, secrets)),
    [api, saveProfile],
  );

  const updateProfile = useCallback(
    async (profile: VPNProfile, secrets: VPNSecrets) => {
      const result = await saveProfile(() => api.saveVPNProfile(profile, secrets));
      if (result.saved) setEditingProfile("");
      return result;
    },
    [api, saveProfile],
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

      {overview.available ? null : <VPNUnavailableNotice overview={overview} />}
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
                {editingProfile === name ? (
                  <VPNProfileForm
                    busy={operation.busy}
                    editing={status.profile}
                    onSave={updateProfile}
                    onCancel={() => setEditingProfile("")}
                  />
                ) : (
                  <VPNProfileCard
                    status={status}
                    busy={operation.busy}
                    available={overview.available}
                    actions={{
                      onStart: () => void startProfile(name),
                      onStop: () => void act(() => api.stopVPNSession(name)),
                      onShowLogs: () => setShownLogs(name),
                      onEdit: () => setEditingProfile(name),
                      onRename: () => setPendingRename(name),
                      onRemove: () => setPendingRemoval(name),
                    }}
                  />
                )}
              </li>
            );
          })}
        </ul>
      )}

      <VPNProfileForm busy={operation.busy} onSave={createProfile} />

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
          validate={(value) => {
            // engine と同じ規則で、名前の誤りをこの欄に出す。
            const refused = vpnProfileNameError(value);
            return refused === null ? "" : describeVPNFieldError(t, refused);
          }}
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

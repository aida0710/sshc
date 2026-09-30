import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { vpnApi, type VPNApi, type VPNOverview, type VPNProfile, type VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import type { NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { useDraftDiscardConfirmation } from "../routing/useDraftDiscardConfirmation";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import { hintText } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { usePolling } from "../ui/usePolling";
import { describeVPNFieldError, vpnFieldErrorOf, type VPNFieldError } from "./vpnFieldErrors";
import { VPNLogsDialog } from "./VPNLogsDialog";
import { VPNProfileCard } from "./VPNProfileCard";
import { VPNProfileForm, type VPNProfileSaveResult } from "./VPNProfileForm";
import { overviewRefreshIntervalMs, routeProgressIntervalMs } from "./vpnOverviewPolling";
import { describeVPNProblem } from "./vpnProblemMessage";
import { vpnProfileNameError } from "./vpnProfileRules";
import { VPNUnavailableNotice } from "./VPNUnavailableNotice";

// 接続ごとのVPN経路の画面。トンネルはengineが持つコンテナの中にあり、ここでは
// プロファイルの定義と、いまの状態と、どの接続がそれを使うかを扱う。接続へ
// プロファイルを付けるのは Connections で行う。シークレットは保存のときに送り、編集を
// 開いたときだけ、確認のトークンを添えてengineから取り出す。

type VPNPanelProps = {
  api?: VPNApi;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => void) | undefined;
};

export function VPNPanel({ api = vpnApi, onNavigationBlockerChange, onNavigateLocation }: VPNPanelProps) {
  const t = useTranslate();
  const operation = useAsyncOperation();
  const [overview, setOverview] = useState<VPNOverview | null>(null);
  const [pendingRemoval, setPendingRemoval] = useState("");
  // pendingDisconnect は、切断を確かめている経路と、それを使っている接続の数である。
  const [pendingDisconnect, setPendingDisconnect] = useState<{ name: string; openConnections: number } | null>(null);
  const [pendingRename, setPendingRename] = useState("");
  // editingProfile は、いま編集のフォームを開いているプロファイルである。
  const [editingProfile, setEditingProfile] = useState("");
  const [shownLogs, setShownLogs] = useState("");
  // startingProfiles は、この画面がいま経路を用意させているプロファイルである。用意は
  // 初回のイメージの作成で分単位になるので、ほかのプロファイルの操作は止めない。
  const [startingProfiles, setStartingProfiles] = useState<ReadonlySet<string>>(() => new Set());
  // abandonedStarts は、用意の途中で利用者が切断したプロファイルである。sshcエンジンは
  // その用意を打ち切るので、打ち切られた失敗は知らせない。
  const abandonedStarts = useRef(new Set<string>());
  // failedProfile は、直前に経路を用意できなかったプロファイルである。失敗の文の
  // 横からそのログを開けるようにする。
  const [failedProfile, setFailedProfile] = useState("");
  // 編集と作成のフォームに、保存していない入力があるか。画面を離れると入力は消える。
  const [editFormDirty, setEditFormDirty] = useState(false);
  const [createFormDirty, setCreateFormDirty] = useState(false);
  const draftDiscard = useDraftDiscardConfirmation({
    dirty: editFormDirty || createFormDirty,
    onNavigationBlockerChange,
    onNavigateLocation,
  });

  // 別のプロファイルの編集を開くと、いま開いている編集のフォームは閉じて入力が消えるので、
  // その入力を変えていれば先に確かめる。作成のフォームは残るので、その入力では確かめない。
  function openEditor(name: string) {
    if (editFormDirty) draftDiscard.confirmBefore(() => setEditingProfile(name));
    else setEditingProfile(name);
  }

  // 一覧を書き換えた応答の世代。操作を始めるときと終えるときに進め、読み始めたあとに
  // 世代が変わった応答は捨てる。遅れて届いたポーリングの応答が、操作の運んだ新しい
  // 一覧を古い一覧で上書きしないためである。
  const overviewGeneration = useRequestGeneration();

  const describe = useCallback((error: unknown) => describeVPNProblem(t, error) ?? t("vpn.failed"), [t]);

  const { run, fail, clearError } = operation;
  // act は、一覧を返す操作を、画面全体を待たせて走らせる。応答の一覧は、その間に別の操作が
  // 始まっていなければ採る。失敗の言い方は describeFailure で変えられる。成功したかどうかを返す。
  const act = useCallback(
    async (work: () => Promise<VPNOverview>, describeFailure: (error: unknown) => string = describe) => {
      const isCurrent = overviewGeneration.begin();
      const succeeded = await run(work, {
        apply: (next) => {
          if (isCurrent()) setOverview(next);
        },
        describe: describeFailure,
      });
      if (isCurrent()) overviewGeneration.retire();
      return succeeded;
    },
    [describe, overviewGeneration, run],
  );

  // 開いたときは、経路の状態を docker から読むのを待たずに一覧を読む。docker は
  // mac では1回に1秒前後かかる。経路の状態は、下の読み直しが埋める。以降は、操作の
  // 応答と読み直しが最新の一覧を運ぶ。
  const loadOverview = useCallback(
    () => void act(() => api.vpnOverview({ waitForRoutes: false })),
    [act, api],
  );

  useEffect(() => {
    loadOverview();
  }, [loadOverview]);

  // refresh は、一覧を読み直す。読み始めたあとに操作が始まっていれば、その応答の方が
  // 新しいので、読んだ一覧は捨てる。
  const refresh = useCallback(() => {
    const isCurrent = overviewGeneration.observe();
    return api
      .vpnOverview()
      .then((next) => {
        if (isCurrent()) setOverview(next);
      })
      .catch(() => undefined);
  }, [api, overviewGeneration]);

  // 経路の状態をまだ確かめていない一覧が届いたら、すぐに確かめに行く。
  const checking = overview?.checking ?? false;
  useEffect(() => {
    if (checking) void refresh();
  }, [checking, refresh]);

  // startProfile は、経路を用意させる。ほかの操作と違い、画面全体を待たせない。
  const startProfile = useCallback(
    async (name: string) => {
      setStartingProfiles((current) => new Set(current).add(name));
      abandonedStarts.current.delete(name);
      clearError();
      // 応答の一覧は、act と同じく、その間に別の操作が始まっていなければ採る。
      const isCurrent = overviewGeneration.begin();
      try {
        const next = await api.startVPNRoute(name);
        if (isCurrent()) setOverview(next);
      } catch (error) {
        if (!abandonedStarts.current.has(name)) {
          if (failureCode(error) === "vpn_route_failed") setFailedProfile(name);
          fail(describe(error));
        }
      } finally {
        if (isCurrent()) overviewGeneration.retire();
        abandonedStarts.current.delete(name);
        setStartingProfiles((current) => {
          const next = new Set(current);
          next.delete(name);
          return next;
        });
      }
    },
    [api, clearError, describe, fail, overviewGeneration],
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

  const disconnectProfile = useCallback(
    (name: string) => {
      if (startingProfiles.has(name)) abandonedStarts.current.add(name);
      void act(() => api.disconnectVPNRoute(name));
    },
    [act, api, startingProfiles],
  );

  // requestDisconnect は、経路を切断する。経路を使っている接続があれば、それらも
  // 切れることを先に確かめる。
  const requestDisconnect = useCallback(
    (name: string, openConnections: number) => {
      if (openConnections > 0) {
        setPendingDisconnect({ name, openConnections });
        return;
      }
      disconnectProfile(name);
    },
    [disconnectProfile],
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

  // 経路の状態は、この画面の外でも変わる。画面を開いているあいだは読み直し、
  // 経路を用意しているあいだは、どこまで進んだかを見せるために間隔を縮める。
  // ほかの操作の最中は読み直さない。その応答が一覧を運んでくるからである。経路を
  // 用意させている最中だけは、その応答が経路が立つまで返らないので、段階を見せる
  // ために読み直し続ける。
  const preparingRoute = overview?.profiles.some((status) => (status.phase ?? "") !== "") ?? false;
  const startingAny = startingProfiles.size > 0;
  const showingProgress = startingAny || preparingRoute || checking;
  usePolling(
    refresh,
    {
      intervalMs: showingProgress ? routeProgressIntervalMs : overviewRefreshIntervalMs,
      enabled: startingAny || !operation.busy,
    },
  );

  if (overview === null) {
    return (
      <PanelState
        tone={operation.error === "" ? "loading" : "failed"}
        title={operation.error === "" ? t("vpn.loading") : operation.error}
        {...(operation.error === ""
          ? {}
          : { action: <Button onClick={loadOverview}>{t("shell.bootstrapRetry")}</Button> })}
      />
    );
  }

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("vpn.heading")} description={t("vpn.description")} />

      {overview.available || overview.checking ? null : <VPNUnavailableNotice overview={overview} />}
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
                    revealSecrets={api.revealVPNSecrets}
                    onSave={updateProfile}
                    onCancel={() => setEditingProfile("")}
                    onDirtyChange={setEditFormDirty}
                  />
                ) : (
                  <VPNProfileCard
                    status={status}
                    busy={operation.busy}
                    starting={startingProfiles.has(name)}
                    available={overview.available}
                    checking={overview.checking}
                    actions={{
                      onStart: () => void startProfile(name),
                      onStop: () => requestDisconnect(name, status.openConnections),
                      onShowLogs: () => setShownLogs(name),
                      onEdit: () => openEditor(name),
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

      <VPNProfileForm busy={operation.busy} onSave={createProfile} onDirtyChange={setCreateFormDirty} />

      {draftDiscard.confirming ? (
        <DiscardDraftDialog id="vpn-draft-discard-heading" onConfirm={draftDiscard.confirmDiscard} onCancel={draftDiscard.keepEditing} />
      ) : null}

      {pendingDisconnect === null ? null : (
        <ConfirmDialog
          id="vpn-disconnect"
          heading={t("vpn.disconnectTitle", { name: pendingDisconnect.name })}
          body={
            <p className="text-sm text-ink-muted">
              {t("vpn.disconnectBody", { count: pendingDisconnect.openConnections })}
            </p>
          }
          confirmLabel={t("vpn.disconnect")}
          cancelLabel={t("vpn.cancel")}
          onCancel={() => setPendingDisconnect(null)}
          onConfirm={() => {
            const { name } = pendingDisconnect;
            setPendingDisconnect(null);
            disconnectProfile(name);
          }}
        />
      )}

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

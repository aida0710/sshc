import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { vpnApi, type VPNApi, type VPNOverview } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { hintText } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { VPNLogsDialog } from "./VPNLogsDialog";
import { VPNProfileForm } from "./VPNProfileForm";
import { vpnRefusals } from "./vpnRefusals";
import { VPNSessionCard } from "./VPNSessionCard";

// 接続ごとのVPN経路の画面。トンネルはengineが持つコンテナの中にあり、ここでは
// 経路の定義と、いまの状態と、どの接続がそれを通るかを扱う。秘密は保存のときに
// 送るだけで、engineは決して返さない。

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

  const describe = useCallback(
    (error: unknown) => {
      const code = failureCode(error);
      const key = code === undefined ? undefined : vpnRefusals[code];
      return key === undefined ? t("vpn.failed") : t(key);
    },
    [t],
  );

  const run = operation.run;
  useEffect(() => {
    // 開いたときに一度だけ読む。以降は、操作の応答が最新の一覧を運ぶ。
    void run(() => api.vpnOverview(), { apply: setOverview, describe });
  }, [api, run, describe]);

  const act = useCallback(
    async (work: () => Promise<VPNOverview>) => {
      await operation.run(work, { apply: setOverview, describe });
    },
    [describe, operation],
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
      {operation.error === "" ? null : <Notice tone="danger">{operation.error}</Notice>}

      {overview.profiles.length === 0 ? (
        <p className={hintText}>{t("vpn.empty")}</p>
      ) : (
        <ul className="flex flex-col gap-3">
          {overview.profiles.map((session) => {
            const name = session.profile.name;
            return (
              <li key={name}>
                <VPNSessionCard
                  session={session}
                  aliases={aliases}
                  busy={operation.busy}
                  available={overview.available}
                  actions={{
                    onStart: () => void act(() => api.startVPNSession(name)),
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
        onSave={(profile, secrets) => void act(() => api.saveVPNProfile(profile, secrets))}
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

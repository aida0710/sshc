import type { VPNSession } from "../api/vpn";
import { useLanguage, useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { formatDateTime } from "../ui/format";
import { Button, Card } from "../ui/surface";
import { VPNBindingRow } from "./VPNBindingRow";
import { vpnPhases } from "./vpnPhases";

// 経路ひとつぶんの札。いまの状態と、コンテナの中のトンネルの様子と、
// この経路を通る接続を見せ、開始・停止・改名・削除・ログを受け付ける。

export type VPNSessionActions = {
  onStart: () => void;
  onStop: () => void;
  onRename: () => void;
  onShowLogs: () => void;
  onRemove: () => void;
  onBind: (alias: string) => void;
  onUnbind: (alias: string) => void;
};

export function VPNSessionCard({
  session,
  aliases,
  busy,
  available,
  actions,
}: {
  session: VPNSession;
  aliases: string[];
  busy: boolean;
  // available は、この機械が経路を作れるかどうかである。
  available: boolean;
  actions: VPNSessionActions;
}) {
  const t = useTranslate();
  const phase = vpnPhases[session.phase ?? ""];
  const state =
    session.relaySocket !== ""
      ? t("vpn.stateUp")
      : phase !== undefined
        ? t(phase)
        : session.running
          ? t("vpn.stateStarting")
          : t("vpn.stateStopped");
  return (
    <Card as="article" padded aria-label={session.profile.name}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="font-medium text-ink">{session.profile.name}</p>
          <p className={hintText}>
            {session.profile.backend} · {session.profile.target} · {state}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || !available} onClick={actions.onStart}>
            {t("vpn.connect")}
          </Button>
          <Button disabled={busy || !session.running} onClick={actions.onStop}>
            {t("vpn.disconnect")}
          </Button>
          <Button disabled={busy || !available} onClick={actions.onShowLogs}>
            {t("vpn.logs")}
          </Button>
          <Button disabled={busy} onClick={actions.onRename}>
            {t("vpn.rename")}
          </Button>
          <Button kind="danger" disabled={busy} onClick={actions.onRemove}>
            {t("vpn.remove")}
          </Button>
        </div>
      </div>

      {session.tunnel === undefined ? null : <TunnelDetail tunnel={session.tunnel} />}

      <VPNBindingRow
        profile={session.profile.name}
        connections={session.connections}
        aliases={aliases}
        busy={busy}
        onBind={actions.onBind}
        onUnbind={actions.onUnbind}
      />
    </Card>
  );
}

// TunnelDetail は、コンテナの中のトンネルが名乗っているものを見せる。繋がらない
// ときに、経路がどこまでできているかを利用者が自分で読める場所である。
function TunnelDetail({ tunnel }: { tunnel: NonNullable<VPNSession["tunnel"]> }) {
  const t = useTranslate();
  const { locale } = useLanguage();
  const rows: { label: string; value: string }[] = [
    { label: t("vpn.tunnelInterface"), value: tunnel.interface ?? "" },
    { label: t("vpn.tunnelAddress"), value: tunnel.address ?? "" },
    {
      label: t("vpn.tunnelSince"),
      value: tunnel.since === undefined || tunnel.since === "" ? "" : formatDateTime(tunnel.since, locale),
    },
    { label: t("vpn.tunnelTargetAddress"), value: tunnel.targetAddress ?? "" },
  ].filter((row) => row.value !== "");
  if (rows.length === 0) return null;
  return (
    <dl className="flex flex-wrap gap-x-6 gap-y-1 border-t border-line pt-3 text-sm">
      {rows.map((row) => (
        <div key={row.label} className="flex gap-2">
          <dt className={hintText}>{row.label}</dt>
          <dd className="font-mono text-ink">{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

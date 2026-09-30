import type { VPNProfile, VPNProfileStatus } from "../api/vpn";
import { useLanguage, useTranslate } from "../i18n/context";
import { hintText, sectionHeading } from "../ui/form";
import { formatDateTime } from "../ui/format";
import { Button, Card } from "../ui/surface";
import { vpnBackendLabel } from "./vpnBackends";
import { vpnPhases } from "./vpnPhases";

// VPNプロファイルひとつぶんの札。いまの状態と、コンテナの中のトンネルの様子と、
// このプロファイルを使う接続を見せ、接続・切断・ログ・編集・名前の変更・削除を受け付ける。
// 接続へプロファイルを付けたり外したりするのは Connections で行い、ここでは見せるだけにする。

export type VPNProfileActions = {
  onStart: () => void;
  onStop: () => void;
  onShowLogs: () => void;
  onEdit: () => void;
  onRename: () => void;
  onRemove: () => void;
};

// serverOf は、プロファイルの VPN サーバーを返す。どのプロファイルかを見分ける手がかりにする。
// OpenVPN と WireGuard は設定ファイルのサーバーが複数ありうるので、並べて見せる。
function serverOf(profile: VPNProfile): string {
  return profile.wireguard?.servers.join(", ") ?? profile.l2tp?.server ?? profile.openconnect?.server ??
    profile.openvpn?.servers.join(", ") ?? profile.ikev2?.server ?? "";
}

export function VPNProfileCard({
  status,
  busy,
  starting,
  available,
  checking,
  actions,
}: {
  status: VPNProfileStatus;
  // busy は、画面全体を待たせる操作（保存、名前の変更、削除、切断）の最中であることを表す。
  busy: boolean;
  // starting は、この画面がこのプロファイルの経路を用意させている最中であることを表す。
  starting: boolean;
  // available は、このマシンが経路を作れるかどうかである。
  available: boolean;
  // checking は、経路の状態をまだ確かめていないことを表す。確かめるまでは、状態を
  // 見せず、接続と切断も受け付けない。
  checking: boolean;
  actions: VPNProfileActions;
}) {
  const t = useTranslate();
  const phase = vpnPhases[status.phase ?? ""];
  // 確かめる前の状態は、sshcエンジンが覚えているものだけで、コンテナが終わったことは
  // 分からない。用意の途中の段階を除いて、確かめるまでは状態を言わない。
  const state =
    checking && phase === undefined
      ? t("vpn.stateChecking")
      : status.relaySocket !== ""
        ? t("vpn.stateUp")
        : phase !== undefined
          ? t(phase)
          : status.running
            ? t("vpn.stateStarting")
            : t("vpn.stateStopped");
  const summary = [vpnBackendLabel(status.profile.backend), serverOf(status.profile), state].filter((part) => part !== "");
  // preparing は、この画面かほかの入口（CLI、Terminal）が経路を用意している最中である。
  // 用意の途中でも切断でき、sshcエンジンは用意を打ち切る。
  const preparing = starting || phase !== undefined;
  return (
    <Card as="article" padded aria-label={status.profile.name}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="font-medium text-ink">{status.profile.name}</p>
          <p className={hintText}>{summary.join(" · ")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || preparing || checking || !available} onClick={actions.onStart}>
            {t("vpn.connect")}
          </Button>
          <Button disabled={busy || checking || !(status.running || preparing)} onClick={actions.onStop}>
            {t("vpn.disconnect")}
          </Button>
          <Button disabled={busy || !available} onClick={actions.onShowLogs}>
            {t("vpn.logs")}
          </Button>
          <Button disabled={busy || starting} onClick={actions.onEdit}>
            {t("vpn.edit")}
          </Button>
          <Button disabled={busy || starting} onClick={actions.onRename}>
            {t("vpn.rename")}
          </Button>
          <Button kind="danger" disabled={busy || starting} onClick={actions.onRemove}>
            {t("vpn.remove")}
          </Button>
        </div>
      </div>

      {status.tunnel === undefined || checking ? null : <TunnelDetail tunnel={status.tunnel} />}

      <ProfileConnections connections={status.connections} />
    </Card>
  );
}

// TunnelDetail は、コンテナの中のトンネルが名乗っているものを見せる。繋がらない
// ときに、経路がどこまでできているかを利用者が自分で読める場所である。
function TunnelDetail({ tunnel }: { tunnel: NonNullable<VPNProfileStatus["tunnel"]> }) {
  const t = useTranslate();
  const { locale } = useLanguage();
  const rows: { label: string; value: string }[] = [
    { label: t("vpn.tunnelInterface"), value: tunnel.interface ?? "" },
    { label: t("vpn.tunnelAddress"), value: tunnel.address ?? "" },
    {
      label: t("vpn.tunnelSince"),
      value: tunnel.since === undefined || tunnel.since === "" ? "" : formatDateTime(tunnel.since, locale),
    },
  ].filter((row) => row.value !== "");
  if (rows.length === 0) return null;
  return (
    <dl className="flex flex-wrap gap-x-6 gap-y-1 border-t border-line pt-3 text-sm">
      {rows.map((row) => (
        <div key={row.label} className="flex items-center gap-2">
          <dt className={hintText}>{row.label}</dt>
          <dd className="font-mono text-ink">{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

// ProfileConnections は、このプロファイルを使う接続の alias を並べる。
function ProfileConnections({ connections }: { connections: string[] }) {
  const t = useTranslate();
  return (
    <div className="flex flex-col gap-2 border-t border-line pt-3">
      <p className={sectionHeading}>{t("vpn.connections")}</p>
      {connections.length === 0 ? (
        <p className={hintText}>{t("vpn.noConnections")}</p>
      ) : (
        <ul className="flex flex-wrap gap-2" aria-label={t("vpn.connections")}>
          {connections.map((alias) => (
            <li key={alias} className="rounded bg-surface-subtle px-2 py-1 text-sm text-ink">
              {alias}
            </li>
          ))}
        </ul>
      )}
      <p className={hintText}>{t("vpn.connectionsHint")}</p>
    </div>
  );
}

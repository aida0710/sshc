import type { VPNProfile, VPNProfileStatus } from "../api/vpn";
import { useLanguage, useTranslate } from "../i18n/context";
import { ActionMenu, revealOnHover, type ActionMenuItem } from "../ui/ActionMenu";
import { tableHeadRow } from "../ui/form";
import { formatDateTime } from "../ui/format";
import { Spinner } from "../ui/Spinner";
import { Button } from "../ui/surface";
import { vpnBackendLabel } from "./vpnBackends";
import { vpnPhases } from "./vpnPhases";

// VPNプロファイルの一覧の1行と、その列の見出し。行は、方式とサーバー・いまの状態とコンテナの中の
// トンネル・このプロファイルを使う接続を見せる。接続と切断は行のボタンで、ログ・編集・名前の変更・
// 削除は「…」で受け付ける。動いている経路の［接続］は、編集で変わった設定で経路を作り直させる。接続へプロファイルを付けたり外したりするのは Connections で行う。

// VPNProfileRowAction は、行から始め、画面全体を待たせる操作である。名前の変更と削除は
// ダイアログを閉じてから始まるので、待つ間は行の「…」の場所に回転の印を出す。
export type VPNProfileRowAction = "disconnect" | "rename" | "remove";

export type VPNProfileActions = {
  onStart: () => void;
  onStop: () => void;
  onShowLogs: () => void;
  onEdit: () => void;
  onRename: () => void;
  onRemove: () => void;
};

// profileColumns は、見出しと行が揃えて使う列である（名前・状態・使う接続・操作）。見出しと行は別々の
// グリッドなので、中身で幅の決まる列を使わない。状態と操作の列は幅を決め、残りを名前と接続で分ける。
// 板が狭いと名前の列が残らないので、行を札にする。画面の幅でなく板の幅で決めるのは、PCではナビが
// 252pxを取るからである。
const profileColumns = "@2xl:grid @2xl:grid-cols-[minmax(0,1.2fr)_11rem_minmax(0,1fr)_12rem] @2xl:gap-x-4";

const tag = "inline-flex h-5 items-center gap-1.5 whitespace-nowrap rounded-full px-[7px] text-[11.5px]";
const neutralTag = `${tag} bg-well text-ink-muted`;
const goodTag = `${tag} bg-live/15 text-live`;

// VPNProfileListHeader は、行の列の名前を小さく薄く並べる。狭い画面では各行が自分で名前を添える。
export function VPNProfileListHeader() {
  const t = useTranslate();
  return (
    <div aria-hidden="true" className={`hidden pb-2 ${tableHeadRow} ${profileColumns}`}>
      <span className="px-2.5">{t("vpn.name")}</span>
      <span className="px-2.5">{t("vpn.columnState")}</span>
      <span className="px-2.5">{t("vpn.connections")}</span>
      <span />
    </div>
  );
}

// serverOf は、プロファイルの VPN サーバーを返す。どのプロファイルかを見分ける手がかりにする。
// OpenVPN と WireGuard は設定ファイルのサーバーが複数ありうるので、並べて見せる。
function serverOf(profile: VPNProfile): string {
  return profile.wireguard?.servers.join(", ") ?? profile.l2tp?.server ?? profile.openconnect?.server ??
    profile.openvpn?.servers.join(", ") ?? profile.ikev2?.server ?? "";
}

export function VPNProfileRow({
  status,
  busy,
  busyWith,
  starting,
  available,
  checking,
  editing,
  actions,
}: {
  status: VPNProfileStatus;
  // busy は、画面全体を待たせる操作（保存、名前の変更、削除、切断）の最中であることを表す。
  busy: boolean;
  // busyWith は、その操作をこの行から始めて待っているかを答える。
  busyWith: (action: VPNProfileRowAction) => boolean;
  // starting は、この画面がこのプロファイルの経路を用意させている最中であることを表す。
  starting: boolean;
  // available は、このマシンが経路を作れるかどうかである。
  available: boolean;
  // checking は、経路の状態をまだ確かめていないことを表す。確かめるまでは、状態を
  // 見せず、接続と切断も受け付けない。
  checking: boolean;
  // editing は、この行の下に編集のフォームを開いていることを表す。
  editing: boolean;
  actions: VPNProfileActions;
}) {
  const t = useTranslate();
  const name = status.profile.name;
  const phase = vpnPhases[status.phase ?? ""];
  // preparing は、この画面かほかの入口（CLI、Terminal）が経路を用意している最中である。
  // 用意の途中でも切断でき、sshcエンジンは用意を打ち切る。
  const preparing = starting || phase !== undefined;
  const up = status.relaySocket !== "";
  // 確かめる前の状態は、sshcエンジンが覚えているものだけで、コンテナが終わったことは
  // 分からない。用意の途中の段階を除いて、確かめるまでは状態を言わない。
  const state =
    checking && phase === undefined
      ? t("vpn.stateChecking")
      : up
        ? t("vpn.stateUp")
        : phase !== undefined
          ? t(phase)
          : status.running || starting
            ? t("vpn.stateStarting")
            : t("vpn.stateStopped");
  const summary = [vpnBackendLabel(status.profile.backend), serverOf(status.profile)].filter((part) => part !== "");
  // 押せない理由のうち、行のほかの所に書いていないものを title で伝える。確かめている
  // 最中であることは状態の文が、このマシンで使えないことは画面の上の知らせが伝える。
  const waitingForOperation = busy ? t("vpn.waitingForOperation") : undefined;
  const waitingToChange = waitingForOperation ?? (starting ? t("vpn.waitingForRoute") : undefined);
  // 編集を開いている行の編集・名前の変更・削除は、フォームの入力を確かめずに消すので、
  // フォームを保存するか取り消すまで押せない。
  const waitingForEdit = editing ? t("vpn.finishEditFirst") : undefined;
  const menuPending = busyWith("rename") || busyWith("remove");
  const menu: ActionMenuItem[] = [
    { label: t("vpn.logs"), disabled: busy || !available, title: waitingForOperation, onSelect: actions.onShowLogs },
    { label: t("vpn.edit"), disabled: busy || starting || editing, title: waitingToChange ?? waitingForEdit, onSelect: actions.onEdit },
    { label: t("vpn.rename"), disabled: busy || starting || editing, title: waitingToChange ?? waitingForEdit, onSelect: actions.onRename },
    { label: t("vpn.remove"), tone: "danger", disabled: busy || starting || editing, title: waitingToChange ?? waitingForEdit, onSelect: actions.onRemove },
  ];
  return (
    <article
      aria-label={name}
      aria-busy={menuPending || busyWith("disconnect") || undefined}
      className={`group/row grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 py-3 hover:bg-hover @2xl:py-[9px] ${profileColumns}`}
    >
      <div className="min-w-0 @2xl:px-2.5">
        <p className="text-[13.5px] text-ink [overflow-wrap:anywhere]">{name}</p>
        <p className="text-[11.5px] text-ink-faint [overflow-wrap:anywhere]">{summary.join(" · ")}</p>
      </div>
      <div className="col-span-2 row-start-2 flex min-w-0 flex-col items-start gap-1 @2xl:col-span-1 @2xl:row-start-auto @2xl:px-2.5">
        <span className={up ? goodTag : neutralTag}>
          {preparing && !up ? <Spinner className="size-2.5" /> : null}
          {state}
        </span>
        {status.tunnel === undefined || checking ? null : <TunnelDetail tunnel={status.tunnel} />}
      </div>
      <div className="col-span-2 row-start-3 min-w-0 @2xl:col-span-1 @2xl:row-start-auto @2xl:px-2.5">
        <ProfileConnections connections={status.connections} />
      </div>
      <div className="col-start-2 row-start-1 flex items-center justify-end gap-1 @2xl:col-start-auto @2xl:row-start-auto @2xl:pr-1">
        <Button
          small
          busy={starting}
          disabled={busy || preparing || checking || !available}
          title={starting ? undefined : waitingForOperation}
          onClick={actions.onStart}
        >
          {t("vpn.connect")}
        </Button>
        <Button
          small
          busy={busyWith("disconnect")}
          disabled={busy || checking || !(status.running || preparing)}
          title={busyWith("disconnect") ? undefined : waitingForOperation}
          onClick={actions.onStop}
        >
          {t("vpn.disconnect")}
        </Button>
        {menuPending ? (
          <span className="flex size-10 items-center justify-center text-ink-muted @2xl:size-7">
            <Spinner />
          </span>
        ) : (
          <ActionMenu
            label={t("vpn.rowActions", { name })}
            items={menu}
            triggerClassName={`flex size-10 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink @2xl:size-7 ${revealOnHover}`}
          />
        )}
      </div>
    </article>
  );
}

// TunnelDetail は、コンテナの中のトンネルが名乗っているものを見せる。繋がらない
// ときに、経路がどこまでできているかを利用者が自分で読める場所である。列が狭いので
// 値だけを並べ、項目の名前は読み上げと title で伝える。
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
    <dl className="flex min-w-0 flex-wrap gap-x-2 font-mono text-[11px] leading-4 text-ink-faint">
      {rows.map((row) => (
        <div key={row.label} title={row.label} className="min-w-0 [overflow-wrap:anywhere]">
          <dt className="sr-only">{row.label}</dt>
          <dd>{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

// ProfileConnections は、このプロファイルを使う接続の alias を並べる。
function ProfileConnections({ connections }: { connections: string[] }) {
  const t = useTranslate();
  return (
    <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
      <span className="text-[11.5px] text-ink-faint @2xl:hidden">{t("vpn.connections")}</span>
      {connections.length === 0 ? (
        <span className="text-xs text-ink-faint">{t("vpn.noConnections")}</span>
      ) : (
        <ul className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5" aria-label={t("vpn.connections")}>
          {connections.map((alias) => (
            <li key={alias} className="font-mono text-xs text-ink-muted [overflow-wrap:anywhere]">
              {alias}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

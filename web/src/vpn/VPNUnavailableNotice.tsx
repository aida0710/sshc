import type { VPNOverview } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Notice } from "../ui/surface";
import { vpnRefusalMessages } from "./vpnRefusals";

// このマシンで VPN 経路を使えない理由を見せる。理由は engine が返す語を訳して出し、
// docker の生の文（英語）は訳さずに「詳細」として小さく添える。訳せない原因を調べる
// ときに、その文がそのまま手がかりになるからである。
export function VPNUnavailableNotice({ overview }: { overview: VPNOverview }) {
  const t = useTranslate();
  const reason = overview.unavailable === undefined ? t("vpn.unavailable") : t(vpnRefusalMessages[overview.unavailable]);
  const detail = overview.detail ?? "";
  return (
    <Notice>
      <span className="flex min-w-0 flex-col gap-1">
        <span>{reason}</span>
        {detail === "" ? null : (
          <span className="break-words text-xs opacity-80">{t("vpn.unavailableDetail", { detail })}</span>
        )}
      </span>
    </Notice>
  );
}

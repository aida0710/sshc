import type { MessageKey } from "../i18n/messages";

// 経路を用意している段階と、その画面での言い方。engine の段階名と同じ語を使う。
export const vpnPhases: Record<string, MessageKey> = {
  image: "vpn.phaseImage",
  container: "vpn.phaseContainer",
  tunnel: "vpn.phaseTunnel",
};

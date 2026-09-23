import type { MessageKey } from "../i18n/messages";

// engine が VPN の操作を断った理由と、その画面での言い方。
export const vpnRefusals: Record<string, MessageKey> = {
  vpn_docker_missing: "vpn.dockerMissing",
  vpn_tunnel_device_missing: "vpn.tunnelDeviceMissing",
  vpn_image_build_failed: "vpn.imageBuildFailed",
  vpn_session_failed: "vpn.sessionFailed",
  vpn_container_foreign: "vpn.containerForeign",
  vpn_secrets_missing: "vpn.secretsMissing",
  vpn_profile_invalid: "vpn.profileInvalid",
  vpn_profile_unknown: "vpn.profileUnknown",
  vpn_profile_exists: "vpn.profileExists",
  vpn_target_mismatch: "vpn.targetMismatch",
  connection_unknown: "vpn.connectionUnknown",
  vault_locked: "vpn.vaultLocked",
};

package vpnrefusal

import "sshc/internal/vpn"

// CLI が使う英語の言い方である。文面は画面の英語（web/src/i18n/messages/en.ts の
// vpn.*）に揃え、CLI で打つコマンドを案内する日本語の文には、同じコマンドを添える。
// backend に固有のものは、その backend の file の init が足す。

// english は、CLI が使う英語の言い方である。
var english = phrasebook{
	codes: englishSentences, fields: englishFieldReasons, destinations: englishDestinationReasons,
	routes: englishRouteReasons, targets: englishTargetReasons, directives: englishDirectiveReasons,
	frames: phraseFrames{
		routeFailed:     "Connecting to the VPN failed. %s",
		targetFailed:    "The destination could not be reached through the VPN. %s",
		operationFailed: "The VPN operation failed.",
		secretsMissing:  "No secrets (private key or password) are stored for this VPN profile.",
		profileInvalid:  "The VPN profile has a value that cannot be used. Check the VPN server and the DNS servers.",
		lineDirective:   "Line %d: %s",
		line:            "Line %d: %s",
		peerMissing:     "The [Peer] on line %d has no \"%s\".",
		fileMissing:     "The configuration file has no \"%s\".",
		fieldSeparator:  ": ",
	},
}

// englishSentences は、理由の語を持たない拒否の言い方である（sentences の英語）。
var englishSentences = map[string]string{
	CodeProfileUnknown: "There is no VPN profile by that name. Check the names with sshc vpn.",
	CodeProfileExists:  "A VPN profile with that name already exists.",
	CodeConnectionUnknown: "This connection cannot have a VPN profile. Connections defined only in an Included " +
		"file or by a wildcard Host cannot be edited by sshc.",
	CodeVaultLocked:  "The vault is locked. Unlock it with sshc vault unlock, then try again.",
	CodeVaultMissing: "There is no vault yet. Create one with sshc vault create, then try again.",
	CodeDockerMissing: "Docker was not found. VPN routes need Docker. " +
		"Install Docker Desktop or similar.",
	CodeDockerNotRunning: "Docker is not running. Start Docker Desktop or similar, then try again. " +
		"If it is running, check that the current user may use Docker.",
	CodeTunnelDeviceMissing: "Docker cannot use the tunnel device this type needs (/dev/net/tun, /dev/ppp).",
	CodeImageBuildFailed:    "Building the VPN container image failed. Check the network and Docker.",
	CodeContainerForeign: "A container of the same name was created by something other than sshc, " +
		"so the operation was stopped.",
	CodeSocketPathTooLong: "The path where sshc keeps its data (~/.ssh/sshc) is too long for the VPN route's relay. " +
		"Move ~/.ssh to a folder with a shorter path and put a symbolic link in its place.",
	CodeChangedConcurrently: "Another operation changed the same settings at the same time, so nothing was saved. " +
		"Try again.",
	CodeRouteDisconnected: "The VPN route was disconnected, so starting it was cancelled.",
	CodeRouteStopped:      "The VPN route was stopped, so starting it was cancelled.",
}

// englishFieldReasons は、項目を受け取れなかった理由の言い方である（fieldReasons の英語）。
var englishFieldReasons = map[vpn.Reason]string{
	vpn.ReasonRequired:    "Enter a value.",
	vpn.ReasonFormat:      "This is not written in a form this field accepts.",
	vpn.ReasonTooLong:     "Too long: up to %d characters.",
	vpn.ReasonTooMany:     "Too many: up to %d.",
	vpn.ReasonOutOfRange:  "The port must be between 1 and 65535.",
	vpn.ReasonNotIPv4:     "Not an IPv4 address.",
	vpn.ReasonUnroutable:  "No route can be built to this address (loopback, multicast or unspecified).",
	vpn.ReasonUnsupported: "This value cannot be used.",
	vpn.ReasonUnexpected:  "This setting is not used by the chosen type.",
}

// englishDestinationReasons は、接続先を VPN 経由で使えない理由の言い方である
// （destinationReasons の英語）。
var englishDestinationReasons = map[vpn.Reason]string{
	vpn.ReasonFormat:     "The destination's HostName is not in a valid form.",
	vpn.ReasonOutOfRange: "The destination's Port must be between 1 and 65535.",
	vpn.ReasonNotIPv4: "A destination through a VPN must be an IPv4 address or a host name. " +
		"IPv6 addresses are not supported.",
	vpn.ReasonUnroutable:   "This address (loopback, multicast or unspecified) cannot be used through a VPN.",
	vpn.ReasonNameNeedsDNS: "To use a host name as the destination, give the VPN profile a DNS server inside the VPN.",
}

// englishRouteReasons は、経路を用意できなかった理由の言い方である（routeReasons の英語）。
var englishRouteReasons = map[vpn.FailureReason]string{
	vpn.FailureUnknown:           "The reason could not be read. Check the logs.",
	vpn.FailureTimeout:           "The connection timed out.",
	vpn.FailureServerUnresolved:  "The VPN server's name could not be resolved. Check how the server is written.",
	vpn.FailureIPsecNegotiation:  "IPsec negotiation failed. Check the pre-shared key and the cipher suites.",
	vpn.FailurePPPAuthentication: "PPP authentication failed. Check the username and the password.",
	vpn.FailureOpenConnect:       "Check the username, the password, the second factor and the certificate.",
	vpn.FailureHandshakeTimeout:  "The handshake failed. Check the keys and the server.",
	vpn.FailureTunnelLost:        "The VPN was disconnected right after the connection was established.",
}

// englishTargetReasons は、経路はあるが接続先へ繋げなかった理由の言い方である
// （targetReasons の英語）。
var englishTargetReasons = map[vpn.FailureReason]string{
	vpn.FailureTargetUnresolved: "The DNS servers inside the VPN could not resolve the destination. " +
		"Check the DNS servers and HostName.",
	vpn.FailureTargetNeedsDNS: englishDestinationReasons[vpn.ReasonNameNeedsDNS],
	vpn.FailureTargetIsServer: "The destination is the VPN server's own address. " +
		"The VPN server itself cannot be reached through the VPN.",
	vpn.FailureTargetUnreachable: "The destination did not answer or refused the connection. Check HostName and Port.",
	vpn.FailureTunnelLost:        "The VPN was disconnected. Connect again.",
	vpn.FailureTimeout:           englishRouteReasons[vpn.FailureTimeout],
}

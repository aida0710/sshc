// The keywords of ssh_config(5) as people write them, for completing a keyword in an SSH config field
// (sshConfigLanguage.ts). OpenSSH reads them without regard to case; internal/config's openSSHKeywordNames is the
// list it knows, and internal/config/web_keywords_test.go checks that every one here is in it. Keywords OpenSSH reads
// only to warn that they are gone are left out.
export const sshConfigKeywords: readonly string[] = [
  "AddKeysToAgent", "AddressFamily", "BatchMode", "BindAddress", "BindInterface",
  "CanonicalDomains", "CanonicalizeFallbackLocal", "CanonicalizeHostname", "CanonicalizeMaxDots",
  "CanonicalizePermittedCNAMEs", "CASignatureAlgorithms", "CertificateFile", "ChannelTimeout", "CheckHostIP",
  "Ciphers", "ClearAllForwardings", "Compression", "ConnectionAttempts", "ConnectTimeout", "ControlMaster",
  "ControlPath", "ControlPersist", "DynamicForward", "EnableEscapeCommandline", "EnableSSHKeysign", "EscapeChar",
  "ExitOnForwardFailure", "FingerprintHash", "ForkAfterAuthentication", "ForwardAgent", "ForwardX11",
  "ForwardX11Timeout", "ForwardX11Trusted", "GatewayPorts", "GlobalKnownHostsFile", "GSSAPIAuthentication",
  "GSSAPIDelegateCredentials", "HashKnownHosts", "Host", "HostbasedAcceptedAlgorithms", "HostbasedAuthentication",
  "HostKeyAlgorithms", "HostKeyAlias", "HostName", "IdentitiesOnly", "IdentityAgent", "IdentityFile",
  "IgnoreUnknown", "Include", "IPQoS", "KbdInteractiveAuthentication", "KbdInteractiveDevices", "KexAlgorithms",
  "KnownHostsCommand", "LocalCommand", "LocalForward", "LogLevel", "LogVerbose", "MACs", "Match",
  "NoHostAuthenticationForLocalhost", "NumberOfPasswordPrompts", "ObscureKeystrokeTiming",
  "PasswordAuthentication", "PermitLocalCommand", "PermitRemoteOpen", "PKCS11Provider", "Port",
  "PreferredAuthentications", "ProxyCommand", "ProxyJump", "ProxyUseFdpass", "PubkeyAcceptedAlgorithms",
  "PubkeyAuthentication", "RekeyLimit", "RemoteCommand", "RemoteForward", "RequestTTY", "RequiredRSASize",
  "RevokedHostKeys", "SecurityKeyProvider", "SendEnv", "ServerAliveCountMax", "ServerAliveInterval", "SessionType",
  "SetEnv", "StdinNull", "StreamLocalBindMask", "StreamLocalBindUnlink", "StrictHostKeyChecking", "SyslogFacility",
  "Tag", "TCPKeepAlive", "Tunnel", "TunnelDevice", "UpdateHostKeys", "User", "UserKnownHostsFile",
  "VerifyHostKeyDNS", "VersionAddendum", "VisualHostKey", "XAuthLocation",
];

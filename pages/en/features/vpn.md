---
title: Per-connection VPN
description: Route chosen SSH connections through a VPN of their own without touching the host default route or DNS.
---

# Per-connection VPN

Route a chosen SSH connection through a VPN that belongs to that connection alone. The host default route, DNS and VPN client stay as they are. **The host can stay on one VPN while sshc reaches a host that only exists inside another.**

A VPN profile holds only what it takes to reach the VPN: its settings and secrets. It has no target. Like a password, a profile is attached to connections in Connections, and each connection reaches its own `HostName` and `Port` through it. One profile can serve many connections.

This needs Docker on the machine. When Docker is missing or not running, sshc says so and refuses rather than falling back.

## What happens

```text
sshc engine
   │ one docker exec per connection (standard input and output)
   ▼
relay inside the VPN container
   │ TCP connection to the target
   ▼
tunnel ─ VPN ─ Docker's ordinary uplink ─ VPN server
```

The SSH handshake, authentication, host-key checks and ssh-agent all run in the engine on the host. The container is told where to open a TCP connection and nothing else: no SSH private key and no vault key reach it.

For every connection the engine starts the container's relay with `docker exec` and uses its standard input and output as the connection. No socket is shared between the host and the container, so the same design works on Linux and on Docker Desktop for macOS and Windows.

The container adds a route and a packet filter only for the targets connections actually use; it never pulls in the network behind the VPN. Traffic to a target is fail closed. The container rejects packets to the target that would leave through anything but the tunnel, so while the tunnel is down nothing reaches the target over Docker's ordinary uplink.

## What "does not touch the host" covers

The host default route, DNS, NetworkManager and an already-connected VPN are left alone. Neither `--privileged` nor `--network host` is used. The container gets one tunnel device (`/dev/net/tun` for WireGuard, OpenConnect and OpenVPN, `/dev/ppp` for L2TP/IPsec; none for IKEv2/IPsec, which uses the kernel's XFRM interface) and only the capabilities that backend needs; for WireGuard and OpenVPN everything but `CAP_NET_ADMIN`, `CAP_NET_RAW`, `CAP_DAC_OVERRIDE` and `CAP_CHOWN` is dropped.

It is not unrelated to the host: it uses Docker's bridge and the kernel's tunnel support, and anyone who can drive Docker generally holds strong host privileges. What is isolated is the VPN's routes, DNS and connection state, and the application traffic sent into it.

## Creating a profile

Create one from the VPN screen or with `sshc vpn add <name>`.

| Field | Meaning |
|---|---|
| Type | WireGuard, L2TP/IPsec, OpenConnect (Cisco AnyConnect, ocserv, GlobalProtect, FortiGate, Ivanti Connect Secure and more), OpenVPN, or IKEv2/IPsec |
| DNS inside the VPN | Only for connections whose `HostName` is a name. Up to three IPv4 addresses |
| VPN server | `host:port` for WireGuard, a hostname or address for L2TP/IPsec, OpenConnect and IKEv2/IPsec. OpenVPN reads it from `remote` in the configuration file |
| Secrets | A private key for WireGuard; the VPN password and IPsec pre-shared key for L2TP/IPsec; the VPN password for OpenConnect; the configuration file (.ovpn) and, when it asks for one, the VPN password for OpenVPN; the VPN password or the IPsec pre-shared key for IKEv2/IPsec |

Secrets are kept in the vault. They are never returned to the screen or the API.

Creating and editing are separate operations. Creating a profile whose name is already taken is refused and changes nothing: to change its settings, use **Edit** on the VPN screen or `sshc vpn edit <name>`; to change only its name, rename it. If the vault still holds a secret under the same name, creating replaces it with the secret you entered instead of inheriting it.

When editing, a secret left empty keeps its stored value, so settings can be changed without entering the secrets again. Changing the type drops the old type's secrets and asks for the new type's. `sshc vpn edit` starts every prompt from the saved value, and `-` clears an optional setting. Settings and secrets are saved in one write; one is never changed without the other.

A value that cannot be accepted is reported with the field and the reason (missing, wrongly written, over a limit and so on), both on the screen and by `sshc vpn add`.

For L2TP/IPsec, set IKE and ESP proposals only when an older device rejects the defaults.

For OpenConnect, choose the product the VPN server runs; leave Cisco AnyConnect if unsure.

| Product | Protocol |
|---|---|
| Cisco AnyConnect / Secure Client, ocserv | `anyconnect` |
| Palo Alto Networks GlobalProtect | `gp` |
| Fortinet FortiGate | `fortinet` |
| Ivanti Connect Secure (Pulse Secure) | `pulse` |
| Juniper Network Connect | `nc` |
| F5 BIG-IP | `f5` |
| Array Networks | `array` |

Sign-in through a browser (SAML and other single sign-on) is not supported. Servers that accept a user name and password, optionally with a one-time password or approval on a phone, work.

 A device with a self-signed certificate needs its fingerprint, either `sha256:` (the certificate's own SHA-256, in hex) or `pin-sha256:` (the public key pin, in base64); without one the certificate is verified normally and the route is refused if it does not verify. OpenConnect routes and DNS handed out by the device are not installed: only the routes to the targets your connections use are.

### The second factor (Duo Mobile and friends)

When the device asks one more question after the password, set the second factor.

| Choice | What is sent | When |
|---|---|---|
| None | Nothing | A device that asks once |
| Wait for approval on the phone | Nothing, or the word you give | Duo Mobile approval |
| Generate a code from a stored seed | A fresh six-digit code | A device where a TOTP seed can be enrolled |

Duo devices come in two shapes. Some **push the notification as soon as the password is accepted** and hold the response until you approve; others **ask one more question** that takes a word such as `push`. For the first, leave the word blank. Only the second needs it.

Either way the route is given two minutes, which is what noticing a notification, unlocking the phone and approving it takes. While it waits, the VPN screen and `sshc vpn` say it is waiting for approval on the phone. When the route is started by connecting to a host (the terminal, or `sshc <host>` on the command line), the connection also prints an `[sshc]` line saying so, whatever the connection log setting.

The TOTP seed is kept in the vault and the code is generated just before it is handed to the container. Neither the seed nor the code appears in `docker logs` or on screen.

A setup that requires a browser-based SAML login, such as Duo's Universal Prompt, is not supported. There is no browser in the container and the engine does not stand in for you.

### OpenVPN configuration files

OpenVPN uses the client configuration file (.ovpn) that a provider or an organization hands out. On the VPN screen, paste the file or load it with **Choose a file**; `sshc vpn add` asks for its path.

A configuration file usually carries certificates and keys, so its contents are kept in the vault as a secret. The VPN servers shown in the list are read from its `remote` lines.

A configuration file can be used when it:

- has `client` (or `tls-client`) and at least one `remote`
- embeds its certificates and keys, as in `<ca>`…`</ca>`, `<cert>`, `<key>`, `<tls-auth>` and `<tls-crypt>`
- uses a `tun` tunnel (`dev tap` is not supported)
- keeps every line within 254 bytes and the whole file within 64 KiB

A username and password are optional. Enter them when the file has `auth-user-pass`; leave them blank when a certificate alone authenticates. The password reaches OpenVPN through a file in memory (tmpfs) inside the container, never through a command-line argument or an environment variable, and `--auth-nocache` keeps OpenVPN from holding on to it after use.

#### Directives that are refused

For safety, a configuration file with any of the following directives cannot be saved. The screen and `sshc vpn add` name the line and the directive; remove that line and load the file again.

| Kind | Directives | Why |
|---|---|---|
| Runs a command or loads a program | `up`, `down`, `route-up`, `route-pre-down`, `ipchange`, `tls-verify`, `client-connect`, `client-disconnect`, `client-crresponse`, `learn-address`, `auth-user-pass-verify`, `tls-crypt-v2-verify`, `iproute`, `plugin`, `engine`, `pkcs11-providers`, `script-security` | Anything could run inside the container and undo the rule that only the target's traffic goes into the tunnel |
| Changes routes or DNS | `route`, `route-ipv6`, `redirect-gateway`, `redirect-private`, `client-nat` | sshc adds a route for each target itself |
| Decided by sshc | `dev` and `dev-type` other than `tun`, `dev-node`, `lladdr`, `mktun`, `rmtun`, `daemon`, `log`, `log-append`, `syslog`, `status`, `writepid`, `tmp-dir`, `chroot`, `cd`, `user`, `group`, `setcon`, `askpass`, and every directive starting with `management` | sshc decides the tunnel interface, where the log goes and how OpenVPN runs |
| Names a file | `ca`, `cert`, `key`, `tls-auth`, `tls-crypt`, `pkcs12`, `auth-user-pass` and the like with a file name, and `config`, `capath`, `tls-export-cert`, `replay-persist`, `genkey` | The file is not in the container. Embedding its contents, as in `<ca>`…`</ca>`, works |
| For a VPN server | `mode`, `server`, `server-ipv6`, `server-bridge`, `tls-server` | sshc only connects as a client |

A directive behind `setenv opt` is checked as if it were written on its own. The list was checked against the OpenVPN 2.6 manual.

#### What sshc decides

Whatever the file says, sshc uses its own values for these:

- the tunnel interface is `tun0`
- routes, the default route and DNS handed out by the server (`redirect-gateway`, `route`, `dhcp-option` and so on) are not taken
- the interface gets its address as a `/32`, and only the targets your connections use get a route
- a failed authentication is not retried
- the log level is `verb 3`

Neither `dhcp-option DNS` in the file nor DNS handed out by the server is used. When a connection's `HostName` is a name, give the profile its DNS servers inside the VPN, as with the other types.

### IKEv2/IPsec

sshc reaches the IKEv2/IPsec servers that the built-in VPN of Windows, macOS and phones connects to. Choose one of two ways to authenticate.

| Authentication | What you enter | How the server is checked |
|---|---|---|
| Username and password (EAP-MSCHAPv2) | The VPN username and the VPN password | Its certificate |
| Pre-shared key (PSK) | The local ID and the IPsec pre-shared key | The pre-shared key |

With a username and password, the server's certificate is verified before the password is sent.

- Paste a PEM certificate into **CA certificate** to trust only certificates that CA issued. Several certificates, intermediate CAs included, can be pasted one after another.
- Leave it blank to verify against the public certificate authorities (those in Ubuntu's `ca-certificates`).
- A server that cannot be verified is never connected to, and the password is never sent to it.

The name in the certificate is checked against **Server ID (remote ID)**, which defaults to the VPN server field. When the VPN server is given as an address but the certificate carries a hostname, give that hostname as the server ID. Values starting with `%`, such as `%any`, are refused.

With a pre-shared key, the server authenticates with the same key. The local ID is the ID the VPN server knows this machine by. No CA certificate is used.

Set IKE and ESP proposals only when the VPN server does not accept the defaults. They are written as for L2TP/IPsec: the given proposals are offered first and the defaults after them, and a trailing `!` offers only the given ones.

The virtual IP address the server hands out is taken, but the routes and DNS servers it hands out are not installed. The tunnel is bound to an IPsec XFRM interface, so even a server that asks for every destination (`0.0.0.0/0`) to go through the tunnel leaves the container's default route alone. As with the other types, only the routes to the targets your connections use are added. A destination the server does not allow is not reached through the tunnel either.

### Naming a target inside the VPN

When a connection's `HostName` is a name, give the profile attached to it the DNS servers that can resolve it. The name is resolved inside the container, using those servers alone: not the host's `resolv.conf`, and not Docker's own DNS. That is what keeps a name that means something else on the host from sending the connection to the wrong machine. With no DNS servers in the profile, a named target is refused; Connections points this out when you pick the profile.

Queries to those servers leave only through the tunnel. The address they returned is written to the container's output (**Logs**, `sshc vpn logs`).

A name is resolved once, the first time a connection uses it after the route opens. If it starts pointing somewhere else afterwards, take the route down and bring it up again.

## Attaching a profile to a connection

As with a password, a profile is attached from the connection's side: pick **VPN profile** on the connection's *sshc settings* tab in Connections, or run `sshc vpn bind <alias> <profile>` (`sshc vpn unbind <alias>` detaches it). Both write the same setting. The VPN screen lists the connections that use each profile. The setting is not written to `~/.ssh/config`: it is an sshc setting, not a word OpenSSH reads.

A connection with a profile takes the same route from the terminal, from SFTP and from `sshc <alias>`, and reaches its own `HostName` and `Port`. When the route is not available the connection is refused rather than quietly sent over the ordinary uplink.

Which connection takes which route is visible before and after connecting. The terminal and SFTP headers show **VPN: \<name\>**, and `sshc info <alias>` prints the same name on its `vpn` line.

Removing a profile also removes its secrets and detaches it from every connection that used it, and stops its route if it is running. Removal needs an unlocked vault; while the vault is locked it is refused and nothing changes. Removing the settings but leaving the secrets behind would let a profile recreated under the same name pick up the old secrets.

To change a name, do not delete and recreate: use **Rename** on the VPN screen or `sshc vpn rename <old name> <new name>`, which moves the settings, the secrets and the connections that use it together. Renaming also needs an unlocked vault. The new name, whether it is taken and the vault are all checked before the route is stopped, so a refused rename leaves a route in use running.

## While a route comes up

`sshc vpn up` and **Connect** on the VPN screen wait until the route is usable. On a machine that has not built the image yet, the first run takes a few minutes. How far it has got is shown in order: building the image, starting the container, waiting for the tunnel. When the route is started by connecting to a host (the terminal, or `sshc <host>` on the command line), the connection prints an `[sshc]` line while the image is being built, whatever the connection log setting.

## How long a route lives

A route opens when it is needed and closes when it is not. The route and packet filter for a target are added the first time a connection uses it. Stopping the engine closes every route it opened. A route with no connection running through it for ten minutes is closed as well. Connections from Terminal, SFTP, `sshc <target>`, and `sshc vpn proxy` are all counted the same way, so a route in use is never closed. A route that was only brought up with `sshc vpn up` or the Connect button counts from the moment it came up. A route whose tunnel dropped ends with its container rather than leaving the relay behind.

If you interrupt a connection with `Ctrl-C` while it is waiting, or close the page, the container that was being prepared does not remain.

Closing a route tells the VPN device first. OpenConnect sends a logout, L2TP/IPsec sends an L2TP disconnect and ends the IPsec session, and IKEv2/IPsec ends the IPsec session. OpenVPN tells the server when the configuration file has `explicit-exit-notify`. No session is left behind on the device, so its concurrent-connection slot is freed as well.

**Disconnect** on the VPN screen closes the route and the connections that use it (Terminal, SFTP, `sshc <target>`). When connections are using the route, the screen shows how many and asks before disconnecting. The VPN screen keeps the state of each route current while it is open, so a route started from another screen or the command line can be disconnected without reopening the screen. When the screen opens, the profiles are listed right away, and each route shows "checking" until its state has been read from Docker.

Terminal does not reconnect the connections closed by **Disconnect** or `sshc vpn down` automatically. Reconnecting them automatically would start the route again right after it was disconnected. Press **Reconnect** in Terminal, or open the connection again, to start the route and connect.

Connecting again reopens the route. One that needs approval on the phone will ask for it again.

## When a route will not come up

While a route is open, the VPN screen and `sshc vpn` show the tunnel's interface, its address inside the VPN and when it opened. If that much is there, the tunnel itself is up. A WireGuard route does not open until the peer has completed a handshake, so a wrong key or server shows a reason instead of an open route.

When a route does not come up, the screen and `sshc vpn up` say why as far as it is known, for example that the WireGuard peer never completed a handshake or that PPP authentication failed. For OpenVPN they tell apart a server that refused the authentication (`AUTH_FAILED`), a failed TLS handshake, and a server that never answered. IKEv2/IPsec tells these apart:

| Message | What to check |
|---|---|
| IKEv2 authentication failed | The username, the password, the pre-shared key and the local ID |
| The VPN server's certificate could not be verified | The server ID and the CA certificate |
| The cipher suite negotiation failed | The IKE and ESP proposals |
| The VPN server does not answer | The VPN server, and that UDP ports 500 and 4500 reach it |
| The IPsec XFRM interface could not be created | Whether Docker's Linux kernel supports XFRM interfaces |

When the VPN is up but the target cannot be reached, the terminal and `sshc <alias>` say why:

| Message | What to check |
|---|---|
| The target's name could not be resolved by the DNS servers inside the VPN | The profile's DNS servers and the connection's `HostName` |
| The target did not answer or refused the connection | The connection's `HostName` and `Port`, and whether its SSH server is running |
| The target is the VPN server itself | The VPN server cannot be reached through its own VPN |

When the failure will repeat until a setting is fixed (no VPN secret saved, Docker not running and so on), the terminal does not keep reconnecting.

If it is not there, or the tunnel is up but the target is still unreachable, read the logs: **Logs** on the VPN screen, or `sshc vpn logs <name>`. They show the sshc engine's record first and the container's output after it. The engine's record lists how the engine prepared the route (which docker it used, the image build, every docker command it ran and the output of the ones that failed), so it also explains a failure that happened before the container started, such as an image that could not be built. A container that failed to come up is cleaned away, but the engine keeps its output, so it can still be read the same way. The stored secrets are replaced by `[REDACTED]`, so the output can be pasted as it is. You never need to run `docker logs` yourself.

## Using the route from the host's `ssh`, `scp` and `git`

Called as a `ProxyCommand`, `sshc vpn proxy` lets any SSH client take the same route. Pass the target as `%h %p`; it is required.

```text
Host lab
  HostName 10.9.9.1
  ProxyCommand sshc vpn proxy tohoku %h %p
```

When the target cannot be reached, it refuses rather than falling back to the ordinary uplink and says why on standard error. The handshake, the keys and `known_hosts` stay with whoever called it; sshc only carries the bytes.

## Limits

- A target is an IPv4 address or a name; IPv6 addresses are not supported. The VPN server itself cannot be a target.
- Not available for hops beyond a jump host: those travel inside the first SSH connection, where this machine's VPN cannot apply.
- A connection with a profile cannot also use `ProxyCommand`, which runs on this machine and is therefore outside the VPN. To reach the route from a `ProxyCommand`, leave the profile off and use `sshc vpn proxy` as shown above.
- Verified on Linux so far. IKEv2/IPsec uses the XFRM interface of Docker's Linux kernel (Linux 4.19 or later).

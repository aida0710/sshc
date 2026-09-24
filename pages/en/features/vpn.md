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

The host default route, DNS, NetworkManager and an already-connected VPN are left alone. Neither `--privileged` nor `--network host` is used. The container gets one tunnel device (`/dev/net/tun` for WireGuard and OpenConnect, `/dev/ppp` for L2TP/IPsec) and only the capabilities that backend needs; for WireGuard everything but `CAP_NET_ADMIN`, `CAP_NET_RAW`, `CAP_DAC_OVERRIDE` and `CAP_CHOWN` is dropped.

It is not unrelated to the host: it uses Docker's bridge and the kernel's tunnel support, and anyone who can drive Docker generally holds strong host privileges. What is isolated is the VPN's routes, DNS and connection state, and the application traffic sent into it.

## Creating a profile

Create one from the VPN screen or with `sshc vpn add <name>`.

| Field | Meaning |
|---|---|
| Type | WireGuard, L2TP/IPsec, or OpenConnect (AnyConnect, ocserv, GlobalProtect and friends) |
| DNS inside the VPN | Only for connections whose `HostName` is a name. Up to three IPv4 addresses |
| VPN server | `host:port` for WireGuard, a hostname or address for L2TP/IPsec and OpenConnect |
| Secrets | A private key for WireGuard; the VPN password and IPsec pre-shared key for L2TP/IPsec; the VPN password for OpenConnect |

Secrets are kept in the vault. They are never returned to the screen or the API.

Creating and editing are separate operations. Creating a profile whose name is already taken is refused and changes nothing: to change its settings, use **Edit** on the VPN screen or `sshc vpn edit <name>`; to change only its name, rename it. If the vault still holds a secret under the same name, creating replaces it with the secret you entered instead of inheriting it.

When editing, a secret left empty keeps its stored value, so settings can be changed without entering the secrets again. Changing the type drops the old type's secrets and asks for the new type's. `sshc vpn edit` starts every prompt from the saved value, and `-` clears an optional setting. Settings and secrets are saved in one write; one is never changed without the other.

A value that cannot be accepted is reported with the field and the reason (missing, wrongly written, over a limit and so on), both on the screen and by `sshc vpn add`.

For L2TP/IPsec, set IKE and ESP proposals only when an older device rejects the defaults.

For OpenConnect, choose the protocol the device speaks (`anyconnect`, `nc`, `pulse`, `gp`, `f5`, `fortinet`, `array`); leave `anyconnect` if unsure. A device with a self-signed certificate needs its fingerprint, either `sha256:` (the certificate's own SHA-256, in hex) or `pin-sha256:` (the public key pin, in base64); without one the certificate is verified normally and the route is refused if it does not verify. OpenConnect routes and DNS handed out by the device are not installed: only the routes to the targets your connections use are.

### The second factor (Duo Mobile and friends)

When the device asks one more question after the password, set the second factor.

| Choice | What is sent | When |
|---|---|---|
| None | Nothing | A device that asks once |
| Wait for approval on the phone | Nothing, or the word you give | Duo Mobile approval |
| Generate a code from a stored seed | A fresh six-digit code | A device where a TOTP seed can be enrolled |

Duo devices come in two shapes. Some **push the notification as soon as the password is accepted** and hold the response until you approve; others **ask one more question** that takes a word such as `push`. For the first, leave the word blank. Only the second needs it.

Either way the route is given two minutes, which is what noticing a notification, unlocking the phone and approving it takes. While it waits, the VPN screen and `sshc vpn` say it is waiting for approval on the phone.

The TOTP seed is kept in the vault and the code is generated just before it is handed to the container. Neither the seed nor the code appears in `docker logs` or on screen.

A setup that requires a browser-based SAML login, such as Duo's Universal Prompt, is not supported. There is no browser in the container and the engine does not stand in for you.

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

`sshc vpn up` and **Connect** on the VPN screen wait until the route is usable. On a machine that has not built the image yet, the first run takes a few minutes. How far it has got is shown in order: building the image, starting the container, waiting for the tunnel.

## How long a route lives

A route opens when it is needed and closes when it is not. The route and packet filter for a target are added the first time a connection uses it. Stopping the engine closes every route it opened. A route with no connection running through it for ten minutes is closed as well. Connections from Terminal, SFTP, `sshc <target>`, and `sshc vpn proxy` are all counted the same way, so a route in use is never closed. A route that was only brought up with `sshc vpn up` or the Connect button counts from the moment it came up. A route whose tunnel dropped ends with its container rather than leaving the relay behind.

If you interrupt a connection with `Ctrl-C` while it is waiting, or close the page, the container that was being prepared does not remain.

Closing a route tells the VPN device first. OpenConnect sends a logout, and L2TP/IPsec sends an L2TP disconnect and ends the IPsec session. No session is left behind on the device, so its concurrent-connection slot is freed as well.

Connecting again reopens the route. One that needs approval on the phone will ask for it again.

## When a route will not come up

While a route is open, the VPN screen and `sshc vpn` show the tunnel's interface, its address inside the VPN and when it opened. If that much is there, the tunnel itself is up. A WireGuard route does not open until the peer has completed a handshake, so a wrong key or server shows a reason instead of an open route.

When a route does not come up, the screen and `sshc vpn up` say why as far as it is known, for example that the WireGuard peer never completed a handshake or that PPP authentication failed.

When the VPN is up but the target cannot be reached, the terminal and `sshc <alias>` say why:

| Message | What to check |
|---|---|
| The target's name could not be resolved by the DNS servers inside the VPN | The profile's DNS servers and the connection's `HostName` |
| The target did not answer or refused the connection | The connection's `HostName` and `Port`, and whether its SSH server is running |
| The target is the VPN server itself | The VPN server cannot be reached through its own VPN |

When the failure will repeat until a setting is fixed (no VPN secret saved, Docker not running and so on), the terminal does not keep reconnecting.

If it is not there, or the tunnel is up but the target is still unreachable, read the container's output: **Logs** on the VPN screen, or `sshc vpn logs <name>`. A container that failed to come up is cleaned away, but the engine keeps its output, so it can still be read the same way. The stored secrets are replaced by `[REDACTED]`, so the output can be pasted as it is. You never need to run `docker logs` yourself.

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
- Verified on Linux so far.

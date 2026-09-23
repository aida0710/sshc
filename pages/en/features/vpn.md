---
title: Per-connection VPN
description: Route chosen SSH connections through a VPN of their own without touching the host default route or DNS.
---

# Per-connection VPN

Route a chosen SSH connection through a VPN that belongs to that connection alone. The host default route, DNS and VPN client stay as they are. **The host can stay on one VPN while sshc reaches a host that only exists inside another.**

This needs Docker on the machine. Without it, sshc says so and refuses rather than falling back.

## What happens

```text
sshc engine
   │ Unix socket (only its owner can open it)
   ▼
relay inside the VPN container
   │ TCP connection to the target
   ▼
tunnel ─ VPN ─ Docker's ordinary uplink ─ VPN server
```

The SSH handshake, authentication, host-key checks and ssh-agent all run in the engine on the host. The container is told where to open a TCP connection and nothing else: no SSH private key and no vault key reach it.

Traffic to the target is fail closed. The container rejects packets to the target that would leave through anything but the tunnel, so while the tunnel is down nothing reaches the target over Docker's ordinary uplink.

## What "does not touch the host" covers

The host default route, DNS, NetworkManager and an already-connected VPN are left alone. Neither `--privileged` nor `--network host` is used. The container gets `CAP_NET_ADMIN` and one tunnel device (`/dev/net/tun` for WireGuard, `/dev/ppp` for L2TP/IPsec).

It is not unrelated to the host: it uses Docker's bridge and the kernel's tunnel support, and anyone who can drive Docker generally holds strong host privileges. What is isolated is the VPN's routes, DNS and connection state, and the application traffic sent into it.

## Creating a profile

Create one from the VPN screen or with `sshc vpn add <name>`. One profile reaches one target, so its route and packet filter close over a single address instead of pulling in a whole network.

| Field | Meaning |
|---|---|
| Type | WireGuard or L2TP/IPsec |
| Target inside the VPN | `host:port`. VPN DNS is not used, so give an IPv4 address |
| VPN server | `host:port` for WireGuard, a hostname or address for L2TP/IPsec |
| Secrets | A private key for WireGuard; the VPN password and IPsec pre-shared key for L2TP/IPsec |

Secrets are kept in the vault. They are never returned to the screen or the API, and settings can be edited without entering them again.

For L2TP/IPsec, set IKE and ESP proposals only when an older device rejects the defaults.

## Binding a connection

Choose a connection on the VPN screen and select **Route through this VPN**, pick **VPN route** on the connection's *sshc settings* tab, or run `sshc vpn bind <alias> <profile>`. All three write the same binding. The binding is not written to `~/.ssh/config`: it is an sshc setting, not a word OpenSSH reads.

A bound connection takes the same route from the terminal, from SFTP and from `sshc <alias>`. When the route is not available the connection is refused rather than quietly sent over the ordinary uplink.

Which connection takes which route is visible before and after connecting. The terminal and SFTP headers show **VPN: \<name\>**, and `sshc info <alias>` prints the same name on its `vpn` line.

Removing a profile also removes its secrets and the bindings of every connection that named it. To change a name, do not delete and recreate: use **Rename** on the VPN screen or `sshc vpn rename <old name> <new name>`, which moves the settings, the secrets and the bindings together.

## While a route comes up

`sshc vpn up` and **Connect** on the VPN screen wait until the route is usable. On a machine that has not built the image yet, the first run takes a few minutes. How far it has got is shown in order: building the image, starting the container, waiting for the tunnel.

## When a route will not come up

While a route is open, the VPN screen and `sshc vpn` show the tunnel's interface, its address inside the VPN and when it opened. If that much is there, the tunnel itself is up.

If it is not there, or the tunnel is up but the target is still unreachable, read the container's output: **Logs** on the VPN screen, or `sshc vpn logs <name>`. The stored secrets are replaced by `[REDACTED]`, so the output can be pasted as it is. You never need to run `docker logs` yourself.

## Limits

- One target per profile, IPv4 only.
- Not available for hops beyond a jump host: those travel inside the first SSH connection, where this machine's VPN cannot apply.
- Cannot be combined with `ProxyCommand`, which runs on this machine and is therefore outside the VPN.
- VPN DNS is not used; targets are given as IPv4 addresses.
- Not available to the host's own `ssh`, `scp` or `git`. Use sshc's terminal, SFTP or CLI.
- Verified on Linux so far.

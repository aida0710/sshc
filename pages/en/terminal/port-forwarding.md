---
title: Port forwarding
description: Manage saved or temporary Local forwarding and Dynamic SOCKS.
---

# Port forwarding

![Local forwarding settings](/images/port-forwarding.png)

sshc provides Local forwarding and Dynamic SOCKS5. Remote forwarding is intentionally not provided.

## Local forwarding

Listen on a local address and port, then connect from the SSH host to a destination host and port.

```text
127.0.0.1:8080  →  SSH host  →  127.0.0.1:80
```

The destination may be any host visible from the SSH server. `127.0.0.1` is the clearest example for a service on that same server.

## Dynamic SOCKS

Open a local SOCKS5 endpoint. Each client request supplies its own destination, so Dynamic settings have no fixed destination field.

Saved forwards live in a connection's sshc tab. A connected terminal can also start and stop temporary forwards on its existing SSH transport.

Listeners are loopback-only, but are not a strong isolation boundary from other local processes or users. Keep authentication enabled on destination services.

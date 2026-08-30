---
title: Jump hosts
description: Use ProxyJump routes and understand per-hop authentication.
---

# Jump hosts

sshc follows `ProxyJump` in process and applies the same host-key and credential policy to every hop.

```ssh-config
Host bastion
  HostName 203.0.113.10
  User ops

Host internal
  HostName 10.0.0.20
  User deploy
  ProxyJump bastion
```

The jump and final host are separate authentication targets. Save the appropriate key, key passphrase, or account password for each alias. Prompts that cannot be answered from saved values, such as 2FA, remain interactive.

Progress and diagnostics identify the exact hop and phase that failed.

The analyzer can display `ProxyCommand`, but sshc's in-process SSH client does not execute arbitrary external commands. Use `ProxyJump` for aliases opened by sshc.

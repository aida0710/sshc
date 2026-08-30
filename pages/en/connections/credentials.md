---
title: Credentials and vault
description: Understand account passwords, key passphrases, and the vault master password.
---

# Credentials and vault

| Secret | Purpose | Scope |
| --- | --- | --- |
| Master password | Opens this device's vault | May differ per device |
| Account password | SSH password authentication | Bound to a connection target |
| Key passphrase | Decrypts a private key | Bound to a key |

Sync uses a separate sync key.

Create labelled account passwords in Secrets and assign them to connections. Editing decrypts the saved value back into the form; leaving the page, locking the vault, or saving discards plaintext UI state.

Key passphrases can be saved, edited, and removed per private key. A ProxyJump route resolves credentials independently for every hop and the final host.

```sh
sshc vault status
sshc vault lock
sshc vault change-password
```

Locking the vault does not close existing SSH sessions, but blocks new secret operations. The vault also locks after 12 hours without activity.

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
| One-time password (TOTP) | Answers a keyboard-interactive verification challenge | Bound to a connection target |

Sync uses a separate sync key.

Create labelled account passwords under **Menu → Vault → Account passwords** and assign them to connections. Editing decrypts the saved value back into the form; leaving the page, locking the vault, or saving discards plaintext UI state.

The assigned password is reused when the same connection is opened from the terminal, SFTP, a ProxyJump route, or the CLI. It does not need to be configured separately for each feature.

Key passphrases can be saved, edited, and removed per private key. A ProxyJump route resolves credentials independently for every hop and the final host.

## Automatic TOTP entry

First, store a Base32 setup key or an `otpauth://totp/...` URI under **Menu → Vault → OTP**. The page normally shows the current six-digit code only; click it to reveal the previous and next code. Then open the target under **Connections** and select the saved token from **Basic → Authentication → One-time password (TOTP)**. You no longer need to type a host alias to create the assignment.

The Vault is split into **Account passwords**, **Key passphrases**, and **OTP** pages so each list and registration form stays focused. The same TOTP entries can be managed from the CLI with `sshc otp list|show|add|edit|remove`.

sshc generates a current code only for a keyboard-interactive question that explicitly says `OTP`, `TOTP`, `Verification code`, or an equivalent phrase. For example, `Verification code:` is recognised. Some servers mark this input as visible; sshc supports that flag without printing the automatically supplied code in the Terminal. It does not send the seed or a code to an ambiguous `Code` prompt or an ordinary password prompt.

The assignment applies to Terminal, SFTP, `sshc ssh`, `sshc run`, and each ProxyJump hop. If the resolved host, user, port, or jump route changes, sshc stops releasing the token so that it cannot be sent to an unintended peer. Select the TOTP again under Basic settings to confirm the new authentication destination.

::: warning Factor separation
Keeping an account password and a TOTP seed in the same device vault is convenient, but a compromised device may expose both factors. Use this only when your organisation's security policy allows it.
:::

```sh
sshc vault status
sshc vault lock
sshc vault change-password
```

Locking the Vault does not close existing SSH sessions, but blocks new secret operations. It locks after 12 hours of inactivity by default; Settings can change the duration or disable automatic locking.

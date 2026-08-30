---
title: Serial and Telnet
description: Use serial consoles, Telnet, legacy encodings, and non-interactive automation.
---

# Serial and Telnet

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

Serial options include data bits, parity, stop bits, flow control, DTR, RTS, and break duration.

```sh
sshc telnet 192.0.2.20:23 --encoding shift_jis
```

Telnet is plaintext and does not authenticate the server. Limit it to a trusted isolated network or another protected boundary.

Serial and Telnet support `utf-8`, `shift_jis`, `euc-jp`, and `iso-2022-jp`. SSH encoding is saved per connection.

## Non-interactive automation

```sh
sshc serial /dev/ttyUSB0 --non-interactive \
  --expect 'login:' --timeout 20s -- 'admin'

sshc telnet 192.0.2.20:23 --non-interactive \
  --script ./steps.json --json -- ''
```

Control expectation or read duration, timeout, settle time, maximum bytes, line endings, and required output. JSON mode returns one result object including warnings.

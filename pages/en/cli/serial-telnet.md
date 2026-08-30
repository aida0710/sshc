---
title: Serial and Telnet
description: Use serial consoles, Telnet, legacy encodings, and non-interactive automation.
---

# Serial and Telnet

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

Serial options include data bits, parity, stop bits, DTR, RTS, and break duration. Flow control currently supports `none` only. The CLI parser accepts `--flow rtscts` and `--flow xonxoff`, but the current system backend rejects them when it opens the connection.

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

Set an expected pattern or a fixed read duration, along with the timeout, settle time, maximum bytes, line endings, and required output. JSON mode returns the result and any warnings in one object.

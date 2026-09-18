---
title: Serial and Telnet
description: Use serial consoles, Telnet, legacy encodings, and non-interactive automation.
---

# Serial and Telnet

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

Serial options include data bits, parity, stop bits, DTR, RTS, and break duration. Flow control supports `none`, `rtscts`, and `xonxoff`. DTR, manual RTS, and break operations are separate from flow control.

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
  --script ./steps.json --json
```

`--script` cannot be combined with `-- <text>`, `--expect`, or `--read-for`: each step in the script file carries its own text and completion condition.

Non-interactive runs need exactly one completion condition: `--expect` succeeds when the regular expression matches, `--read-for` after reading for the given duration. Combine either with the overall `--timeout`, settle time, maximum bytes, line endings, and required output. JSON mode returns the result and any warnings in one object. Exit codes: 124 when the overall timeout elapses, 2 for invalid arguments, 1 for any other failure such as a transport error, and 130 when interrupted.

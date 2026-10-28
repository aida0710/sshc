---
title: Serial and Telnet
description: Use serial consoles, Telnet, legacy encodings, and non-interactive automation.
---

# Serial and Telnet

```sh
sshc serial --json
sshc serial /dev/ttyUSB0 --baud 115200 --encoding utf-8
```

Serial options include data bits, parity, stop bits, DTR, RTS, and break duration. Flow control supports `none`, `rtscts`, and `xonxoff`. `--dtr on|off` and `--rts on|off` are applied once immediately after connecting; `--break <duration>` sends one break of the specified duration at that point. There are no controls to change them during the connection. These options are separate from flow control.

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

Non-interactive runs need exactly one completion condition: `--expect` succeeds when the regular expression matches, `--read-for` after reading for the given duration. Combine either with the overall `--timeout`, settle time, maximum bytes, line endings, and required output. JSON mode returns the result and any warnings in one object. Exit codes: 124 when the overall timeout elapses, 2 for invalid arguments, 1 for any other failure such as a transport error, and 130 when stopped by `Ctrl+C`, SIGTERM, or SIGHUP, including while still connecting.

## Flags

Serial connection:

| Flag | Value |
|---|---|
| `--baud N` | Speed, 1 to 4000000 (default 9600) |
| `--data-bits N` | Data bits: 5, 6, 7 or 8 (default 8) |
| `--parity P` | Parity: `none`, `odd`, `even`, `mark` or `space` |
| `--stop-bits N` | Stop bits: `1`, `1.5` or `2` |
| `--flow F` | Flow control: `none`, `rtscts` or `xonxoff` |
| `--dtr on\|off`, `--rts on\|off` | Set the line once, right after connecting |
| `--break D` | Send one break of this length (up to 5 seconds) right after connecting |

Telnet connection:

| Flag | Value |
|---|---|
| `--connect-timeout D` | How long to wait for the connection (default 10 seconds) |
| `--terminal-type TYPE` | Terminal type to tell the server (1 to 64 bytes) |

Both:

| Flag | Value |
|---|---|
| `--encoding NAME` | Character encoding: `utf-8`, `shift_jis`, `euc-jp` or `iso-2022-jp` |
| `--non-interactive` | Run as automation |
| `--expect REGEX` | Succeed when the output matches this regular expression |
| `--read-for D` | Succeed after reading for this long |
| `--script FILE\|-` | Send each step of a script file (`-` for standard input) in turn |
| `--timeout D` | Overall limit; exceeding it exits with 124 |
| `--settle D` | How long the output must stay quiet before it is taken as complete (0 to 5 seconds, default 120 ms) |
| `--max-bytes N` | Most bytes to read (default 1 MiB, up to 16 MiB) |
| `--line-ending MODE` | Line ending added after sent text: `none`, `cr`, `lf` or `crlf` |
| `--require-output` | Fail when nothing at all was received |
| `--json` | Print the result as JSON. `sshc serial --json` lists serial devices as JSON |

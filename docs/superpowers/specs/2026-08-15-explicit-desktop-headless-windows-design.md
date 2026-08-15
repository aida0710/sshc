# Explicit desktop and headless engine ownership, including Windows

Date: 2026-08-15

Status: approved in conversation

## 1. Purpose

sshc currently has two incompatible stories about who owns the long-lived Go
engine.  The desktop design says that Electron owns the only engine, while a
bare `sshc` invocation can still become a foreground engine.  `--own-engine`
was added later so Electron could refuse that foreground engine instead of
accidentally attaching to it.  The flag prevents two owners from being treated
as one, but it preserves the accidental bare-engine path and makes the public
interface explain an implementation repair rather than a product mode.

This design replaces that ambiguity with two explicit owners:

- Electron owns `sshc engine`.
- A person or an external supervisor owns `sshc headless`.

Both modes run the same Go application and expose the same loopback service,
but their activation, user interaction, and lifetime rules are different.  A
single engine lock makes them mutually exclusive.  A bare `sshc` is a desktop
activation command and never becomes an engine.

This design also promotes Windows to a supported desktop and headless target.
Windows support covers the Go engine and CLI, Electron shell, in-process SSH,
the encrypted vault, the embedded remote terminal, a local ConPTY shell, and a
per-user NSIS installer.  Android remains outside this design.

The process-ownership, bare-invocation, `--own-engine`, headless, and platform
activation decisions here supersede the corresponding decisions in
`2026-08-14-single-app-design.md`.  Unrelated decisions in that document remain
in force.

## 2. Goals

- Give desktop and headless operation distinct, intentional command surfaces.
- Guarantee at most one Go engine for one user's sshc state directory.
- Keep a desktop engine a direct child owned by Electron.
- Keep a headless engine in the foreground and owned by its invoking terminal,
  service manager, container, or multiplexer.
- Make `sshc` and `sshc <alias>` activate and focus the desktop application on
  graphical macOS, Linux, and Windows sessions.
- Let `sshc <alias>` wait for desktop unlock and continue the same connection
  after the user unlocks the application.
- Provide TTY-only CLI operations for creating, unlocking, locking, inspecting,
  and changing the vault master password.
- Preserve the existing eight-hour vault inactivity timeout in both modes.
- Keep a closed desktop window in the background, together with the engine,
  vault state, and active SSH sessions.
- Make app exit, app failure, and headless termination close the engine and its
  active terminal sessions without leaving an orphan.
- Support native build, test, packaging, and smoke-test gates on macOS, Linux,
  and Windows.
- Preserve currently readable SSH configuration, vault, metadata, backup, and
  remote-snapshot data.

## 3. Non-goals

- A built-in daemonizer, detach command, systemd unit writer, launchd agent
  writer, Windows service installer, or background-service toggle.
- Simultaneous desktop and headless engines for one user.
- Electron attaching to or taking over a headless engine.
- A headless engine opening an Electron window.
- Password input through argv, environment variables, ordinary stdin pipes, or
  a `--password-stdin` option.
- Full browser/API administration through new CLI commands in the first phase.
  The phase-one CLI adds engine ownership and the vault operational core while
  retaining existing `connect`, `list`, `open`, and internal `status` commands.
- Compatibility with old command names, old process behavior, old handoff
  documents, or old CLI protocol versions.
- Android.  Android does not provide the Electron, local `~/.ssh`, PTY, and
  unrestricted background-process assumptions used by this application.  A
  future Android client would need a separately designed authenticated remote
  protocol and terminal UI.

## 4. Command surface

The public and internal commands are:

```text
sshc                         launch or focus the desktop application
sshc <alias>                 connect in this terminal through a live engine
sshc connect [text]          choose an alias, then use the same connection flow
sshc list                    list concrete SSH aliases
sshc open                    explicitly print a new browser entrance URL
sshc headless                run the public foreground headless engine
sshc engine                  internal: run the engine owned by the desktop app
sshc status                  print internal engine status JSON
sshc vault status            print human-readable vault and owner state
sshc vault create            create and unlock a new vault
sshc vault unlock            unlock the running engine's vault
sshc vault lock              immediately forget the live vault key
sshc vault change-password   rekey an already-unlocked vault
sshc help                    print usage
```

`--own-engine` is removed without an alias or compatibility shim.  A bare
`sshc` never listens, writes a handoff, or takes `engine.lock`.

`sshc engine` appears in help because this repository does not keep hidden
subcommands, but its description states why it exists and that Electron owns
it.  Exit code 3 remains an internal Electron/Go contract for an engine-owner
conflict; it is not a public scripting API.

These reserved words continue to take precedence over aliases.  A host named
after one of them remains reachable with OpenSSH itself.

## 5. Engine modes and process ownership

### 5.1 Desktop mode

Electron starts exactly one direct Go child:

```text
Electron main process
└── sshc engine
```

Electron gives the child an otherwise empty ownership pipe on stdin and keeps
the write side in the Electron main process only.  Renderer and GPU helpers do
not inherit that handle.  Stdout remains the private entrance channel from the
engine to Electron; stderr remains diagnostic output.

The ownership pipe is a lifetime channel, not authentication and not a command
transport.  `sshc engine` rejects a TTY or absent ownership pipe before taking
the engine lock.  It never polls a parent PID or decides whether a process in a
process table is still the owner.

On normal application exit, Electron closes the ownership channel and waits up
to five seconds for graceful engine shutdown before forcing the child to end.
If Electron is forcibly terminated, the OS closes the write handle.  EOF is a
cancellation notification to the engine, which starts the same shutdown path.
The OS supplies the channel closure; the engine does not infer parent death.

If the Go child exits unexpectedly, Electron cannot remain as a useful shell.
It reports the failure and exits instead of leaving a window or tray item that
cannot serve application state.

Electron has normal Chromium helper processes.  The invariant is not "one OS
process"; it is "one Go engine, directly owned by one Electron main process."

### 5.2 Headless mode

`sshc headless` takes the same engine lock and runs in the foreground.  It does
not receive or require an ownership pipe and does not watch its parent.  Its
lifetime belongs to systemd, tmux, Docker, PowerShell, Task Scheduler, a service
wrapper, or the invoking terminal.

On Unix it handles SIGINT and SIGTERM.  On Windows it handles Ctrl-C and
Ctrl-Break where they are delivered.  It never detaches itself.  A supervisor
that force-kills it still causes the OS to release its listener and lock, though
graceful in-memory and handoff cleanup cannot run.

### 5.3 Mutual exclusion

Both modes take `~/.ssh/sshc/engine.lock` for their entire lifetime:

| Existing owner | Start desktop | Start headless |
| --- | --- | --- |
| none | start | start |
| desktop | reuse/focus existing Electron | refuse |
| headless | refuse and explain how to stop headless | refuse |

macOS and Linux use non-blocking `flock`.  Windows uses non-blocking exclusive
`LockFileEx` on the same per-user state file.  Both mechanisms release their
lock when their process handle closes, including abnormal termination.

The lock, rather than a read-then-start check, is authoritative.  A loser may
briefly wait for the winner to publish a live handoff so it can report the
owner, but it never becomes a second engine and never replaces the winner's
handoff.

Electron's single-instance lock remains the first desktop-shell guard.  The Go
lock remains necessary for concurrent installations, a desktop/headless race,
and direct misuse of the internal engine command.

## 6. Desktop window and background lifetime

Closing the last window does not quit Electron on macOS, Linux, or Windows.
Electron, the ownership channel, the Go engine, unlocked vault state, and live
SSH or local-shell sessions remain in the background.

The application can be reopened by:

- its tray item when the desktop environment supports one;
- Dock, Start Menu, or the desktop application launcher;
- a second desktop invocation, handled by Electron's single-instance event;
- bare `sshc`;
- `sshc <alias>` when desktop unlock is required.

Linux environments without a usable tray still keep the background process.
This deliberately replaces the current behavior that quits with the last
window when tray creation fails.  The application launcher and CLI remain
re-entry paths, so the absence of a tray does not make the process unreachable.

An explicit app/tray Quit requests the number of live terminal sessions from
the engine.  If the count is non-zero, Electron asks once before quitting.
Closing a window never asks because it does not end a session.  Force-ending
Electron skips confirmation and terminates the child through ownership-channel
closure.

The lifetime relationship is:

```text
window lifetime < Electron lifetime = desktop engine lifetime
```

## 7. Desktop activation and stable CLI installation

### 7.1 Shared behavior

Launching and focusing always executes an already identified absolute program
path without a shell.  sshc does not search PATH for a desktop executable and
does not scan a disk for a similarly named application.

If a recorded application has been moved or removed, the CLI explains how to
open or reinstall the application once so its location can be registered
again.  An owner reported by a live authenticated status response always wins
over launcher metadata; a headless owner prevents desktop activation.

### 7.2 macOS

The CLI uses the absolute `/usr/bin/open` and bundle identifier
`com.github.aida0710.sshc`.  User-driven `sshc` activation opens a window and
does not pass `--hidden`.  OS login-item activation may still pass `--hidden`.
A second bundle launch restores and focuses the existing window.

When the desktop application manages the CLI, it atomically copies the bundled
Go binary to `~/.local/share/sshc/bin/sshc` and points
`~/.local/bin/sshc` at that stable managed path.  A source or standalone
installation that placed an unrelated regular file or link at the public path
is not overwritten; the application reports a visible, actionable warning.

### 7.3 Linux

The AppImage uses the same stable managed CLI copy.  It never links
`~/.local/bin/sshc` into the AppImage's temporary mount.

On each packaged AppImage start, Electron atomically records the original,
absolute AppImage path from `APPIMAGE` in
`~/.ssh/sshc/desktop.json`.  It does not record `process.execPath` or a
resource path inside the temporary mount.  The descriptor is private to the
user and contains no credential.

The CLI considers a graphical launch available only when a display is present
(`DISPLAY` or `WAYLAND_DISPLAY`), the descriptor is valid, and the recorded
target is an absolute executable regular file.  It executes that file directly.
A second AppImage instance notifies the existing Electron instance and exits.

Moving or deleting the AppImage produces an instruction to open it once at its
new location.  sshc does not guess among old AppImages.  Login-time startup is
owned by the user's desktop environment, not by sshc-written systemd units.

### 7.4 Windows

Windows is distributed as an unsigned, per-user NSIS installer.  It does not
request administrator privileges.  Its stable layout is:

```text
%LOCALAPPDATA%\Programs\sshc\
├── sshc.exe
└── resources\
    └── cli\
        └── sshc.exe
```

The first executable is Electron.  The second is the Go CLI/engine and is the
same file Electron runs with `engine`.  NSIS adds only the CLI directory to the
user PATH, avoids duplicate entries, and removes only its exact entry on
uninstall.  It records the desktop executable under an sshc-owned HKCU registry
key and removes that key on uninstall.

The CLI reads the absolute desktop path from HKCU and executes it directly.  A
second launch triggers Electron's single-instance focus path.  Missing registry
or executable state tells the user to install or open the NSIS desktop build;
it does not fall back to a PATH search.  Standalone Windows CLI binaries remain
valid for headless use without an NSIS installation.

Windows release targets are x64 and arm64, subject to the verification rule in
section 16: an artifact is not described as supported until it has run on its
target architecture.

## 8. Bare desktop and connection flows

### 8.1 Bare `sshc`

- Live desktop owner: activate or focus the existing Electron window.
- No owner in a graphical session: launch the registered desktop application.
- No owner and no graphical desktop: fail and instruct `sshc headless`.
- Live headless owner: do not launch Electron; instruct the user to stop
  headless before opening the desktop application.

Bare `sshc` never prints a running engine's browser URL.  `sshc open` is the
explicit URL-producing operation.

### 8.2 `sshc <alias>` and `sshc connect`

The CLI first obtains a live, authenticated engine status and then follows the
owner-specific path:

1. Desktop, unlocked: request the connection material and connect immediately
   in the same terminal without focusing the window.
2. Desktop, locked: activate the Electron window, then wait for the same engine
   to become unlocked.
3. No engine, graphical desktop: launch Electron, wait for its handoff, then
   follow the desktop path.
4. Headless, unlocked: request connection material and connect immediately.
5. Headless, locked: fail promptly and instruct `sshc vault unlock`; do not
   wait on an invisible UI.
6. No engine and no graphical desktop: fail and instruct `sshc headless`.

The desktop wait has no arbitrary time limit.  It ends on unlock, Ctrl-C, owner
change, protocol change, or engine exit.  Unlock can occur in the Electron UI
or from another terminal with `sshc vault unlock`; both alter the same engine.
After successful unlock the original CLI resumes the original alias and starts
SSH without asking the user to re-run it.

Failure to reach an engine is no longer permission to connect without stored
answers.  Users who want ordinary OpenSSH behavior without sshc's running vault
use `ssh <alias>`.  Once a live unlocked engine has authorized a connection,
authentication interactions not represented by stored material, such as a
second factor, remain interactive in the terminal.

## 9. Vault CLI

All vault commands operate on a running desktop or headless engine.  They never
open or mutate the vault file in the CLI process and never auto-start an owner.
This keeps vault mutation serialized inside the one engine.

The CLI routes live outside `/api/`, under `/cli/vault/...`, because they are
not browser requests and do not carry a browser cookie, CSRF token, or Fetch
Metadata.  They use the per-run handoff secret and reuse the same service-level
operations as the browser password routes.  Request bodies retain strict size
limits.

Commands that need a password require stdin to be a TTY and use no-echo terminal
input.  They refuse redirected input.  Passwords never appear in argv,
environment variables, command history, normal logs, error payloads, or panic
messages.  `golang.org/x/term`, already a direct dependency, supplies terminal
password input on the supported platforms.

### 9.1 Status

`sshc vault status` needs no TTY and prints human-readable owner and vault state:

```text
engine: headless
vault: locked
```

Owner is `desktop` or `headless`; vault is `missing`, `locked`, or `unlocked`.
The existing `sshc status` remains compact internal JSON for Electron and other
sshc components.

### 9.2 Create

`sshc vault create` refuses an existing vault before prompting, reads a new
master password twice, enforces the existing minimum passphrase length, creates
the vault transactionally, and leaves this engine unlocked.  A confirmation
mismatch performs no write.

### 9.3 Unlock

`sshc vault unlock` reports a missing vault with an instruction to run
`vault create`.  An already-unlocked vault succeeds without prompting.  A
locked vault prompts once and retains the existing increasing refusal delay,
capped at four seconds.  The user-facing failure does not disclose a more
detailed decryption reason.

### 9.4 Lock

`sshc vault lock` immediately forgets the derived vault key and pending secret
retrieval state.  It does not terminate existing SSH sessions, local shells,
PTYs, WebSockets, or port forwards.  Already established sessions have a
different lifecycle; only new secret retrieval becomes unavailable.

### 9.5 Change password

`sshc vault change-password` requires the vault to be unlocked first.  If it is
locked, it instructs `sshc vault unlock` rather than implicitly unlocking and
rekeying in one operation.  It prompts for the current password once and the
new password twice.

The existing transaction rekeys the vault, sealed sync settings, and encrypted
backups.  The same application-level operation attempts to reseal the current
remote snapshot.  A remote reseal failure does not roll back an already
committed local rekey; the CLI reports explicit partial success and its reason.

### 9.6 Startup state and timeout

Desktop and headless engines always start locked.  A successful create or
unlock remains in the engine for the existing eight-hour inactivity interval.
Status reads do not reset that interval.

Headless startup never prompts and never logs an entrance credential.  It
prints one of these non-secret announcements:

```text
sshc headless is running and the vault is locked.
Open another terminal and run:

  sshc vault unlock
```

or:

```text
sshc headless is running and no vault exists.
Open another terminal and run:

  sshc vault create
```

The same text reaches service logs when no terminal is attached.

## 10. Handoff and CLI protocol

The handoff file becomes an atomically written, versioned document:

```json
{
  "schemaVersion": 1,
  "url": "http://127.0.0.1:12345",
  "secret": "per-run random value",
  "owner": "desktop",
  "pid": 1234,
  "version": "v1.0.0",
  "protocolVersion": 1
}
```

The live authenticated `/cli/status` response is authoritative.  The handoff's
owner and PID help routing and diagnostics but do not prove that a process is
alive.  A stale handoff points to a closed listener or a process that does not
know its secret and is treated as no live engine.

CLI and engine protocol versions must match exactly.  This change deliberately
does not carry old protocol compatibility.  A mismatch tells the user to quit
the running app and update sshc; it never falls back to reduced or secretless
behavior.

The handoff is written only after the loopback listener is ready.  Writing uses
a temporary file in the same directory, private permissions or ACL, file
flush, and atomic replacement.  Cleanup removes the file only when its per-run
secret or instance identity still belongs to the stopping engine; matching a
possibly reused URL alone is insufficient.

Unix uses directory mode 0700 and file mode 0600.  Windows applies an ACL that
grants the current user and required system principals only the intended
access.  The handoff remains a same-user boundary: someone who can read it can
already read the encrypted vault and private key material in the same SSH tree.

## 11. Startup and shutdown ordering

Engine startup is:

1. Parse and validate the explicit mode.
2. Validate the Electron ownership channel in desktop mode.
3. Acquire `engine.lock`.
4. Construct services with a locked vault.
5. Bind the 127.0.0.1 listener.
6. Mint the per-run CLI secret.
7. Atomically write the handoff.
8. Begin serving and announce mode-appropriate readiness.

The internal desktop mode writes the one-time bootstrap URL only to its stdout
pipe for Electron.  Headless writes no bootstrap URL at startup.  An explicit
`sshc open` can request and print a new URL from either owner.

Graceful shutdown is:

1. Enter a stopping state and reject new state-changing requests with 503.
2. Remove the handoff if it is still this instance's document.
3. Close live SSH sessions, local shells, WebSockets, and their forwards.
4. Shut down HTTP with a five-second upper bound.
5. Forget the derived vault key and in-memory secret state.
6. Release `engine.lock` last.

Engine exit, unlike `vault lock`, does terminate sessions.  Headless signal
handling does not add an interactive confirmation.  Desktop Quit confirms in
Electron before it closes the ownership channel.  An abnormal process death
may leave a stale handoff, but the OS releases the listener and lock, and the
next legitimate owner atomically replaces the stale document.

## 12. Windows platform foundation

Windows support is not an Electron target flag alone.  The current Windows
cross-build stops at Unix-only `O_NOFOLLOW`, and the present non-Unix engine
lock is a no-op.  Windows receives first-class adapters rather than weakened
guards.

### 12.1 Filesystem safety

Windows file reads use `FILE_FLAG_OPEN_REPARSE_POINT` and reject reparse points
rather than following them.  Atomic replacement uses `MoveFileEx` with
`MOVEFILE_REPLACE_EXISTING` and `MOVEFILE_WRITE_THROUGH`.  Windows-specific
directory flush behavior is documented and tested rather than pretending Unix
directory fsync semantics exist.

Private state, vault, handoff, journals, and backups inherit or receive a
current-user-restricted DACL.  Unix mode assertions are replaced with equivalent
Windows ACL assertions in Windows tests.  Symlink and reparse-point refusal is
not compiled away to make the build pass.

### 12.2 Locking and signals

`LockFileEx` holds the per-user engine file from startup through cleanup.
Windows console cancellation handles Ctrl-C and Ctrl-Break where available.
The Electron ownership channel covers desktop failure.  A foreground headless
process run by a wrapper follows that wrapper's stop/kill semantics.

### 12.3 OpenSSH tools and agent

Connection transport remains the in-process Go SSH client; sshc does not launch
`ssh.exe`.  The only remaining external OpenSSH tool, `ssh-keygen.exe`, is
resolved from fixed trusted Windows installation directories, beginning with
`%WINDIR%\System32\OpenSSH`, without a PATH search.

The Windows OpenSSH agent uses `\\.\pipe\openssh-ssh-agent` and the same
`x/crypto/ssh/agent` protocol used on Unix sockets.  Failure to reach the agent
disables agent-specific operations but does not prevent other authentication.

### 12.4 Local ConPTY shell

Remote embedded SSH sessions are already in-process SSH channels and need no
OS PTY.  Only the embedded local shell needs ConPTY.

Windows uses `CreatePseudoConsole`, `ResizePseudoConsole`, and
`ClosePseudoConsole` from the existing `golang.org/x/sys/windows` dependency.
The target is Windows 10 version 1809 or later.  Shell resolution prefers an
absolute PowerShell 7 `pwsh.exe`, then the system Windows PowerShell, then an
absolute `ComSpec`.  It does not resolve a shell through PATH.

The shell and descendants join a Job Object configured to end them when the
terminal session closes.  This gives Windows the same result as the Unix PTY
process-group SIGHUP rule without pretending Windows has Unix signals.

The existing `x/sys/windows` release already exposes the required ConPTY,
LockFileEx, MoveFileEx, and reparse-point APIs.  No new Go dependency is required
by this design.  If implementation evidence shows a small named-pipe adapter is
safer as a direct module, its manifest and lock data may be added explicitly;
no global or unrelated local package installation is part of the work.

## 13. Compatibility boundary

This release drops compatibility for:

- `--own-engine`;
- bare-engine invocation;
- old engine-management and detach commands;
- old handoff and CLI status documents;
- old owner-selection behavior;
- old CLI protocol versions;
- secretless fallback when the required engine is unavailable.

It preserves currently readable persistent user data:

- `~/.ssh/config` and its Include graph;
- current vault versions and their existing safe upgrade path;
- stored account passwords and key passphrases;
- metadata and terminal settings;
- encrypted transaction backups;
- remote sync settings and snapshots.

Removing current vault or metadata readers would not simplify the process
architecture and would only make existing secrets or configuration unreadable.
"No backward compatibility" applies to executable interfaces and ownership,
not intentional destruction of data that the current version can safely read.

The managed desktop CLI is kept at the same version as its bundle.  If a user
has deliberately installed a separate regular CLI binary, the desktop does not
overwrite it silently; version/protocol mismatch is reported clearly.

## 14. Error and exit contract

Public exit meanings are:

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | operational failure, no engine, locked vault, refused password, or owner conflict |
| 2 | invalid command, argument, or alias |
| 130 | user cancellation with Ctrl-C |

Code 3 is reserved for the internal desktop-engine ownership conflict.

Messages name the next action.  Examples include starting `sshc headless`,
running `sshc vault unlock`, stopping the current headless owner, reopening a
moved AppImage once, or reinstalling the Windows NSIS application.  They do not
silently invoke a different mode.

Vault unlock failures do not expose decryption internals.  Change-password
errors distinguish locked state, rejected current password, local transaction
failure, and remote-snapshot partial failure because those states require
different recovery actions.  No error includes entered password text.

## 15. Clipboard behavior

The already implemented embedded-terminal settings remain part of the supported
desktop behavior:

- selection completion copies once when `copyOnSelect` is enabled;
- right click reads the clipboard and uses xterm's paste path when
  `rightClickPaste` is enabled;
- both settings default to enabled and can be disabled independently;
- changes apply to already-open consoles;
- disabled right-click paste leaves the normal context menu alone;
- clipboard refusal is visible but never logs clipboard contents.

Existing unit and browser E2E coverage remains.  Packaged Electron smoke tests
and the manual platform matrix add macOS, Linux, and Windows system clipboard
evidence.

## 16. Verification and release gates

### 16.1 Native CI

Go build, vet, and tests run natively on Ubuntu, macOS, and Windows.  Race tests
remain mandatory on Linux and macOS and run on Windows when the supported Go
toolchain and runner permit them.  Electron lifecycle tests run on all three
OSes.  Web typechecking, unit tests, generated-file checks, and browser E2E
remain required.

The process integration suite builds a real binary under an isolated temporary
home on every OS and proves:

- bare `sshc` never takes the engine lock;
- headless starts foreground and starts locked;
- missing and locked vault announcements contain no bootstrap secret;
- two headless owners cannot start;
- desktop and headless cannot coexist;
- ownership-channel EOF stops the desktop child;
- headless does not inherit desktop parent-watch behavior;
- abnormal exit releases the OS lock;
- stale handoff replacement and owner-safe cleanup work;
- at most one Go engine remains after every tested race and exit path.

### 16.2 Connection and vault integration

A real controlled sshd verifies desktop-unlocked, desktop-wait-and-unlock,
headless-unlocked, headless-locked, no-engine, Ctrl-C, owner-exit, key
passphrase, account password, ProxyJump-chain password, and remaining
interactive-authentication paths.

Vault integration covers status, create, unlock, lock, change-password,
non-TTY refusal, confirmation mismatch, minimum length, refusal backoff,
eight-hour inactivity, lock-with-live-session, local rekey durability, and
remote-reseal partial success.  Leak tests search argv, environment, request
logs, normal logs, panic output, history, and temporary files for canary secrets.

### 16.3 Platform packaging tests

macOS tests a packaged DMG application, LaunchServices activation, focus,
background window closure, and Cmd+Q child termination.

Linux tests the packaged AppImage on X11 and Wayland where runners are
available, stable CLI installation after AppImage exit, moved-image errors,
single-instance focus, tray-present and tray-absent background behavior, and
reopening through the launcher and CLI.

Windows tests silent per-user NSIS install/uninstall in an isolated runner,
exact user-PATH changes, HKCU launcher registration, PowerShell CLI activation,
LockFileEx, reparse-point refusal, private ACLs, ConPTY start/resize/close, Job
Object descendant cleanup, OpenSSH named-pipe agent behavior, background window
closure, and Electron termination followed by engine termination.

Selection-copy and right-click-paste are exercised in packaged Electron on each
OS in addition to the existing browser suite.

### 16.4 Release workflow

Release production is split by native OS:

- macOS runner builds DMG artifacts;
- Linux runner builds AppImage artifacts;
- Windows runner builds NSIS artifacts;
- each runner smoke-tests what it built;
- a final publish job downloads all successful artifacts, creates checksums,
  and publishes only when every required OS job passed.

Standalone Go binaries are produced for darwin/arm64, darwin/amd64,
linux/arm64, linux/amd64, windows/arm64, and windows/amd64.  Desktop artifacts
are likewise built for the declared architectures.  Cross-compilation alone
does not justify a support claim: README lists an architecture as supported
only after native-runner or real-machine smoke evidence exists.  An unverified
artifact is either withheld or explicitly labeled unverified rather than
silently presented as supported.

The unsigned boundary remains explicit.  macOS may require first-open approval,
and Windows SmartScreen may warn.  Checksums detect transfer corruption but do
not replace signing.

### 16.5 Definition of done

The change is not reported as working on macOS, Linux, and Windows until:

- native CI passes on all three OSes;
- packaged application smoke tests pass on all three OSes;
- the real-sshd connection suite passes;
- desktop/headless exclusion and lifecycle tests pass;
- vault leak and storage-guard tests pass;
- background, explicit quit, and forced-exit behaviors pass;
- generated API and UI bundles match their sources;
- README, help, and design documents no longer contradict actual behavior;
- architecture-level verified and unverified boundaries are stated.

## 17. Documentation changes

README and help are updated in the same implementation to describe:

- the distinction between desktop and headless owners;
- why `sshc engine` exists and why users normally never type it;
- the removal of `--own-engine` and bare engine startup;
- the desktop `sshc` and `sshc <alias>` activation/wait flows;
- TTY-only vault commands and the eight-hour inactivity rule;
- headless foreground examples for systemd, tmux, Docker, and Windows wrappers;
- macOS login items, concrete Linux desktop-environment autostart guidance, and
  Windows Startup Apps guidance;
- AppImage relocation and stable CLI installation;
- NSIS installation, PATH behavior, and SmartScreen warning;
- the supported/verified OS and architecture matrix;
- Android as unsupported rather than merely untested.

Contradictory statements that both retain a bare headless engine and claim that
headless operation was removed are deleted or superseded.  Manual test steps
that treat a bare `sshc` owner blocking Electron as correct are replaced with
the explicit owner matrix in this document.

## 18. Consequences and trade-offs

- Explicit modes add two subcommands, but remove an internal flag and eliminate
  an accidental product mode.
- A headless user must start and unlock a foreground engine explicitly.  This
  is intentional: ordinary passwordless or manually prompted OpenSSH remains
  available as `ssh`.
- Desktop and headless cannot share a live vault or session set.  Refusing is
  simpler and more truthful than attaching Electron to a process it cannot own.
- Keeping Electron alive without a Linux tray can be less discoverable, but the
  application launcher and bare `sshc` provide deterministic re-entry and
  preserve the promised background-session behavior.
- A stable copied CLI means an AppImage or app bundle and its installed CLI
  contain duplicate bytes.  Atomic same-version synchronization is preferable
  to a single symlink that becomes dangling when an AppImage unmounts.
- Windows support requires real platform storage, locking, ACL, agent, and
  ConPTY work.  Compiling those protections away would create a nominal port,
  not supported behavior.
- Retaining existing persistent data readers is deliberate durability, not
  compatibility with the retired process interface.

// Package clispec is the single declarative source for sshc's command tree.
// Parser dispatch, help topics and shell-completion vocabulary are generated
// from this package; semantic flag parsing remains beside each command handler.
package clispec

type Command struct {
	Name    string
	Route   string
	Aliases []string
	Help    string
	Actions []Action
}

type Action struct {
	Name string
	Help string
}

var Commands = []Command{
	{Name: "engine", Route: "engine", Help: `usage:
  sshc engine [--port <n>] [--replace]

Start the engine in the foreground.
  --port <n>  listen on a port from 1024 to 65535
  --replace   stop the running engine first, without asking
`},
	{Name: "ssh", Route: "ssh", Help: `usage:
  sshc ssh
  sshc ssh --list
  sshc ssh <alias>
  sshc ssh <alias> --non-interactive -- <command>

Choose or connect to a Host alias from ~/.ssh/config.
`},
	{Name: "info", Route: "info", Help: `usage:
  sshc info <alias> [--json]

Print the resolved SSH target without connecting.
`},
	{Name: "sync", Route: "sync", Help: `usage:
  sshc sync [--json]
  sshc sync setup
  sshc sync push [--force] [--json]
  sshc sync pull [--force] [--json]
  sshc sync now [--json]
  sshc sync auto on|off [--json]
`, Actions: []Action{
		{Name: "setup", Help: "usage:\n  sshc sync setup\n\nConfigure synchronization in an interactive terminal. Existing values become defaults; blank hidden values keep configured credentials. Direction accepts both, push, or pull.\n"},
		{Name: "push", Help: "usage:\n  sshc sync push [--force] [--json]\n\nPush local synchronization changes through the running engine.\n"},
		{Name: "pull", Help: "usage:\n  sshc sync pull [--force] [--json]\n\nPull synchronization changes through the running engine.\n"},
		{Name: "now", Help: "usage:\n  sshc sync now [--json]\n\nRun one synchronization cycle through the running engine.\n"},
		{Name: "auto", Help: "usage:\n  sshc sync auto on|off [--json]\n\nEnable or disable automatic synchronization.\n"},
	}},
	{Name: "terminal", Route: "terminal", Help: `usage:
  sshc terminal list [--json]
  sshc terminal show <session-id> [--json]
  sshc terminal read <session-id> [--cursor N] [--limit N] [--json]
  sshc terminal send <session-id> --text <text> [--no-enter] [--json]
  sshc terminal wait <session-id> --for <state> [--timeout D] [--json]
  sshc terminal create shell [--json]
  sshc terminal create ssh <alias> [--json]
  sshc terminal rename <session-id> <title> [--json]
  sshc terminal close <session-id> [--json]
`, Actions: []Action{
		{Name: "list", Help: "usage:\n  sshc terminal list [--json]\n\nList terminals owned by the running engine.\n"},
		{Name: "show", Help: "usage:\n  sshc terminal show <session-id> [--json]\n\nShow one terminal. The ID may be a unique lowercase hexadecimal prefix.\n"},
		{Name: "read", Help: "usage:\n  sshc terminal read <session-id> [--cursor N] [--limit N] [--json]\n\nRead retained terminal output from a byte cursor.\n"},
		{Name: "send", Help: "usage:\n  sshc terminal send <session-id> --text <text> [--no-enter] [--json]\n\nSend text to the current process generation. A carriage return is appended\nunless --no-enter is set.\n"},
		{Name: "wait", Help: "usage:\n  sshc terminal wait <session-id> --for <state> [--timeout D] [--json]\n\nStates: connecting, connected, reconnecting, exited, agent-working,\nagent-attention, agent-ready, agent-ended.\n"},
		{Name: "create", Help: "usage:\n  sshc terminal create shell [--json]\n  sshc terminal create ssh <alias> [--json]\n\nCreate a local shell or SSH terminal in the running engine.\n"},
		{Name: "rename", Help: "usage:\n  sshc terminal rename <session-id> <title> [--json]\n\nSet the title of a terminal owned by the running engine.\n"},
		{Name: "close", Help: "usage:\n  sshc terminal close <session-id> [--json]\n\nClose a terminal owned by the running engine.\n"},
	}},
	{Name: "sftp", Route: "sftp", Help: `usage:
  sshc sftp get <alias> <remote-path> <local-path> [options]
  sshc sftp put <alias> <local-path> <remote-path> [options]
  sshc sftp settings [split-options]

Transfer files through the running engine and its SSH/Vault configuration.
Remote paths must be absolute POSIX paths.

Options:
  -r, --recursive   copy directories recursively
  --overwrite       replace existing destination files after confirmation
  --skip-existing   leave existing destination files unchanged
  --dry-run         inspect the transfer plan without changing files
  -j, --jobs <n>    transfer up to 1..8 files in parallel (default 1)
  --split-size <MiB> split files at 16..1024 MiB (engine default 100)
  --split-jobs <n>  use 1..128 streams per large file (engine default 4; 1 disables)
  --chunk-size <MiB> split range size from 8..4096 MiB (engine default 32)
  --json            print one machine-readable result on stdout
  -y, --yes         skip the --overwrite confirmation
`, Actions: []Action{
		{Name: "get", Help: "usage:\n  sshc sftp get <alias> <remote-path> <local-path> [options]\n\nDownload a file or, with --recursive, a directory. Existing files require\n--overwrite and confirmation, or --skip-existing.\n"},
		{Name: "put", Help: "usage:\n  sshc sftp put <alias> <local-path> <remote-path> [options]\n\nUpload a file or, with --recursive, a directory. Existing files require\n--overwrite and confirmation, or --skip-existing.\n"},
		{Name: "settings", Help: "usage:\n  sshc sftp settings [--split-size <MiB>] [--split-jobs <n>] [--chunk-size <MiB>] [--json]\n\nShow the engine-wide split-transfer defaults. Supplied values are persisted and\nused by Web and CLI transfers; get/put flags still override one invocation.\n"},
	}},
	{Name: "serial", Route: "serial", Help: `usage:
  sshc serial [--json]
  sshc serial <device> [options]
  sshc serial <device> [options] --non-interactive [automation] -- <text>

Options: --baud N --data-bits 5..8 --parity none|odd|even|mark|space
         --stop-bits 1|1.5|2 --flow none|rtscts|xonxoff
         --dtr on|off --rts on|off --break D --encoding NAME
Automation: --expect REGEX | --read-for D | --script FILE|-
            --timeout D --settle D --max-bytes N --line-ending MODE
            --require-output --json
`},
	{Name: "telnet", Route: "telnet", Help: `usage:
  sshc telnet <host>[:port] [options]
  sshc telnet <host>[:port] [options] --non-interactive [automation] -- <text>

Options: --connect-timeout D --terminal-type TYPE --encoding NAME
Automation: --expect REGEX | --read-for D | --script FILE|-
            --timeout D --settle D --max-bytes N --line-ending MODE
            --require-output --json
`},
	{Name: "open", Route: "open", Help: "usage:\n  sshc open\n\nPrint a one-time UI URL for the running engine.\n"},
	{Name: "status", Route: "status", Help: "usage:\n  sshc status [--json]\n\nPrint what the running engine is doing.\n"},
	{Name: "update", Route: "update", Help: "usage:\n  sshc update [-y|--yes]\n\nUpdate an installation managed by Homebrew or install.sh. The command shows the plan and asks before changing the installation; -y or --yes skips the prompt.\n"},
	{Name: "service", Route: "service", Help: "usage:\n  sshc service install [-y|--yes]\n  sshc service status\n  sshc service disable [-y|--yes]\n\nManage the sshc engine as a systemd user service on Linux or a launchd user agent on macOS. Mutating actions show the plan and ask for confirmation; -y or --yes skips the prompt.\n", Actions: []Action{
		{Name: "install", Help: "usage:\n  sshc service install [-y|--yes]\n\nInstall and start the sshc user service on Linux or macOS. The command asks for confirmation unless -y or --yes is given.\n"},
		{Name: "status", Help: "usage:\n  sshc service status\n\nPrint whether the sshc-managed user service is active.\n"},
		{Name: "disable", Help: "usage:\n  sshc service disable [-y|--yes]\n\nStop and remove the sshc-managed user service. The command asks for confirmation unless -y or --yes is given.\n"},
	}},
	{Name: "vault", Route: "vault", Help: "usage:\n  sshc vault status\n  sshc vault create\n  sshc vault unlock\n  sshc vault lock\n  sshc vault change-password\n", Actions: []Action{
		{Name: "status", Help: "usage:\n  sshc vault status\n\nDescribe the running engine and Vault.\n"},
		{Name: "create", Help: "usage:\n  sshc vault create\n\nCreate and unlock a new Vault.\n"},
		{Name: "unlock", Help: "usage:\n  sshc vault unlock\n\nUnlock the Vault in the running engine.\n"},
		{Name: "lock", Help: "usage:\n  sshc vault lock\n\nLock the Vault without closing SSH sessions.\n"},
		{Name: "change-password", Help: "usage:\n  sshc vault change-password\n\nChange the password of an unlocked Vault.\n"},
	}},
	{Name: "version", Route: "version", Aliases: []string{"-v", "--version"}, Help: "usage:\n  sshc version\n\nPrint the version and target operating system/architecture.\n"},
	{Name: "help", Route: "help"},
	{Name: "completion", Route: "completion", Help: `usage:
  sshc completion bash|zsh|fish

Print a shell completion script. Host aliases for sshc ssh are read dynamically
from the same ~/.ssh/config and Include files as sshc itself.
`},
	{Name: "-h", Route: "help", Aliases: []string{"--help"}},
}

var Values = map[string][]string{
	"completion-shells":     {"bash", "zsh", "fish"},
	"encodings":             {"utf-8", "shift_jis", "euc-jp", "iso-2022-jp"},
	"wait-states":           {"connecting", "connected", "reconnecting", "exited", "agent-working", "agent-attention", "agent-ready", "agent-ended"},
	"serial-options":        {"--json", "--non-interactive", "--require-output", "--encoding", "--baud", "--data-bits", "--parity", "--stop-bits", "--flow", "--dtr", "--rts", "--break", "--expect", "--read-for", "--timeout", "--settle", "--max-bytes", "--line-ending", "--script", "--help"},
	"telnet-options":        {"--non-interactive", "--require-output", "--encoding", "--connect-timeout", "--terminal-type", "--expect", "--read-for", "--timeout", "--settle", "--max-bytes", "--line-ending", "--script", "--json", "--help"},
	"sftp-options":          {"-r", "--recursive", "--overwrite", "--skip-existing", "--dry-run", "-j", "--jobs", "--split-size", "--split-jobs", "--chunk-size", "--json", "-y", "--yes", "--help"},
	"sftp-settings-options": {"--split-size", "--split-jobs", "--chunk-size", "--json", "--help"},
}

const GlobalHelp = `usage:
  sshc                 open the UI for the running engine
  sshc engine          start the engine in the foreground
                       --port <n>  listen there instead of the preferred port 54447
                       --replace   stop the running engine first, without asking
  sshc ssh [<alias>]   choose a host, or connect to one from ~/.ssh/config
                       --list      print every concrete Host alias
  sshc ssh <alias> --non-interactive -- <command>
                       run an SSH command without an interactive terminal
  sshc completion bash|zsh|fish
                       print shell completion that includes SSH Host aliases
  sshc info <alias> [--json]
                       print the resolved SSH target without connecting
  sshc sync [--json]   print synchronization status from the running engine
  sshc sync setup      configure synchronization in an interactive terminal
  sshc sync push [--force] [--json]
  sshc sync pull [--force] [--json]
  sshc sync now [--json]
  sshc sync auto on|off [--json]
                       run or configure synchronization through the engine
  sshc terminal list [--json]
  sshc terminal show <session-id> [--json]
  sshc terminal read <session-id> [--cursor N] [--limit N] [--json]
  sshc terminal send <session-id> --text <text> [--no-enter] [--json]
  sshc terminal wait <session-id> --for <state> [--timeout D] [--json]
                       states: connecting, connected, reconnecting, exited,
                               agent-working, agent-attention, agent-ready, agent-ended
  sshc terminal create shell [--json]
  sshc terminal create ssh <alias> [--json]
  sshc terminal rename <session-id> <title> [--json]
  sshc terminal close <session-id> [--json]
                       inspect and control terminals owned by the running engine
  sshc sftp get <alias> <remote-path> <local-path> [options]
  sshc sftp put <alias> <local-path> <remote-path> [options]
  sshc sftp settings [split-options]
                       transfer files through the running engine
                       split options: --split-size --split-jobs --chunk-size
                       transfer options: -r --overwrite --skip-existing --dry-run --json -y
  sshc serial [--json]
                       list serial devices
  sshc serial <device> [options]
                       connect interactively to a serial device
                       options: --baud N --data-bits 5..8 --parity none|odd|even|mark|space
                                --stop-bits 1|1.5|2 --flow none|rtscts|xonxoff
                                --dtr on|off --rts on|off --break D --encoding NAME
  sshc telnet <host>[:port] [options]
                       connect interactively with unencrypted Telnet
                       options: --connect-timeout D --terminal-type TYPE --encoding NAME
  sshc serial <device> [options] --non-interactive [automation] -- <text>
  sshc telnet <host>[:port] [options] --non-interactive [automation] -- <text>
                       send text and wait for --expect or --read-for
                       automation: --expect REGEX | --read-for D | --script FILE|-
                                   --timeout D --settle D --max-bytes N --line-ending MODE
                                   --require-output --json
                       encodings: utf-8, shift_jis, euc-jp, iso-2022-jp
  sshc open            print a one-time UI URL
  sshc status          print what the running engine is doing
                       --json      print it as JSON, for the shell
  sshc update [-y]     update an installation managed by Homebrew or install.sh
  sshc service install install and start a user service on Linux or macOS
  sshc service status  print whether the managed service is active
  sshc service disable stop and remove the managed service
  sshc vault status    describe the running engine and vault
  sshc vault create    create and unlock a new vault
  sshc vault unlock    unlock the vault in the running engine
  sshc vault lock      lock the vault without closing SSH sessions
  sshc vault change-password
                       change the password of an unlocked vault
  sshc version         print the version, and what it was built for
  sshc help [<command> ...]
                       print all commands or help for one command

`

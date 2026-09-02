package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

type invocationKind uint8

const (
	invocationInvalid invocationKind = iota
	invocationDesktop
	invocationEngine
	invocationConnect
	invocationRun
	invocationChoose
	invocationList
	invocationOpen
	invocationStatus
	invocationVault
	invocationUpdate
	invocationService
	invocationHelp
	invocationVersion
	invocationTransport
	invocationRunTransport
	invocationInfo
	invocationSync
	invocationTerminal
	invocationSFTP
	invocationCompletion
)

type sftpAction uint8

const (
	sftpInvalid sftpAction = iota
	sftpGet
	sftpPut
	sftpSettings
)

type sftpInvocation struct {
	Action       sftpAction
	Alias        string
	Source       string
	Destination  string
	Recursive    bool
	Overwrite    bool
	SkipExisting bool
	DryRun       bool
	JSON         bool
	Yes          bool
	Jobs         int
	SplitSizeMiB int
	SplitJobs    int
	ChunkSizeMiB int
}

type syncAction uint8

const (
	syncInvalid syncAction = iota
	syncStatus
	syncSetup
	syncPush
	syncPull
	syncNow
	syncAuto
)

type syncInvocation struct {
	Action  syncAction
	Force   bool
	JSON    bool
	Enabled bool
}

type invocation struct {
	Kind invocationKind
	Args []string
	// HelpTopic は個別helpで表示するcommand path。空文字列は全体helpを表す。
	HelpTopic string
	// Port は `sshc engine --port N` の待受ポートで、0 は未指定を表す。
	Port int
	// Replace は既存の engine を確認なしで停止して置き換える。
	Replace bool
	// JSON は `sshc status --json` の機械可読出力を選択する。
	JSON bool
	// Yes は変更内容の確認を省略する。
	Yes       bool
	Transport *transportInvocation
	Sync      *syncInvocation
	Terminal  *terminalInvocation
	SFTP      *sftpInvocation
}

// parseInvocation は、コマンドが誰の責務を求めるかを副作用なしに決める。
//
// alias より先に予約語を一度だけ読むのは、呼び出し側ごとに判定を持つと `engine`
// のような所有者指定が誤って SSH 接続先になり得るためである。ここで確定した Kind
// だけを dispatch すれば、引数の形と実行時の責務がずれない。
func parseInvocation(argv []string) (invocation, error) {
	if len(argv) == 0 {
		return invalidInvocation("missing program name")
	}
	if len(argv) == 1 {
		return invocation{Kind: invocationDesktop}, nil
	}

	word := argv[1]
	args := argv[2:]
	switch generatedCLICommand(word) {
	case cliCommandEngine:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandEngine)), nil
		}
		return parseEngineFlags(args)
	case cliCommandSerial:
		return parseTransportInvocation(transportSerial, args)
	case cliCommandTelnet:
		return parseTransportInvocation(transportTelnet, args)
	case cliCommandSsh:
		return parseSSHInvocation(args)
	case cliCommandInfo:
		return parseInfoInvocation(args)
	case cliCommandSync:
		return parseSyncInvocation(args)
	case cliCommandTerminal:
		return parseTerminalInvocation(args)
	case cliCommandSftp:
		return parseSFTPInvocation(args)
	case cliCommandCompletion:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandCompletion)), nil
		}
		if len(args) != 1 || !validCompletionShell(args[0]) {
			return invalidInvocation("completion requires bash, zsh, or fish")
		}
		return invocation{Kind: invocationCompletion, Args: copyInvocationArgs(args)}, nil
	case cliCommandOpen:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandOpen)), nil
		}
		return noArguments(invocationOpen, word, args)
	case cliCommandStatus:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandStatus)), nil
		}
		if len(args) == 1 && args[0] == "--json" {
			return invocation{Kind: invocationStatus, JSON: true}, nil
		}
		return noArguments(invocationStatus, word, args)
	case cliCommandUpdate:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandUpdate)), nil
		}
		return parseConfirmationOnlyInvocation(invocationUpdate, word, args)
	case cliCommandService:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandService)), nil
		}
		if len(args) > 1 && validServiceAction(args[0]) && isHelpFlag(args[1]) {
			return helpInvocation(canonicalCLICommand(cliCommandService) + " " + args[0]), nil
		}
		if len(args) == 0 || !validServiceAction(args[0]) {
			return invalidInvocation("service requires install, status, or disable")
		}
		action := args[0]
		if action == "status" {
			if len(args) != 1 {
				return invalidInvocation("service status takes no flags")
			}
			return invocation{Kind: invocationService, Args: []string{action}}, nil
		}
		parsed, err := parseConfirmationOnlyInvocation(invocationService, "service "+action, args[1:])
		if err != nil {
			return parsed, err
		}
		parsed.Args = []string{action}
		return parsed, nil
	case cliCommandVault:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandVault)), nil
		}
		if len(args) > 1 && validVaultAction(args[0]) && isHelpFlag(args[1]) {
			return helpInvocation(canonicalCLICommand(cliCommandVault) + " " + args[0]), nil
		}
		if len(args) != 1 {
			return invalidInvocation("vault requires one action")
		}
		switch args[0] {
		case "status", "create", "unlock", "lock", "change-password":
			return invocation{Kind: invocationVault, Args: copyInvocationArgs(args)}, nil
		default:
			return invalidInvocation(fmt.Sprintf("unknown vault action %q", args[0]))
		}
	case cliCommandHelp:
		if word == "-h" || word == "--help" {
			return noArguments(invocationHelp, word, args)
		}
		if len(args) == 0 {
			return helpInvocation(""), nil
		}
		topic := strings.Join(args, " ")
		if !validHelpTopic(topic) {
			return invalidInvocation(fmt.Sprintf("unknown help topic %q", topic))
		}
		return helpInvocation(topic), nil
	// 旗も語も、同じところへ着く。`sshc version` が正式だが、`--version` は
	// 誰もが最初に打つ形である。受けないと、入れた直後の一行目が usage と
	// 終了コード 2 になる。実際 docs/release-install.md はそれを案内していた。
	case cliCommandVersion:
		if word == canonicalCLICommand(cliCommandVersion) && helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandVersion)), nil
		}
		return noArguments(invocationVersion, word, args)
	}

	return invalidInvocation(fmt.Sprintf("unknown command %q", word))
}

func parseSFTPInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandSftp)), nil
	}
	if len(args) > 1 && (args[0] == "get" || args[0] == "put" || args[0] == "settings") && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandSftp) + " " + args[0]), nil
	}
	if len(args) == 0 || (args[0] != "get" && args[0] != "put" && args[0] != "settings") {
		return invalidInvocation("sftp requires get, put, or settings")
	}
	if args[0] == "settings" {
		return parseSFTPSettingsInvocation(args[1:])
	}
	called := sftpInvocation{Action: sftpGet, Jobs: 1}
	jobsSet := false
	splitSizeSet := false
	splitJobsSet := false
	chunkSizeSet := false
	if args[0] == "put" {
		called.Action = sftpPut
	}
	positionals := make([]string, 0, 3)
	for index := 1; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "-r", "--recursive":
			if called.Recursive {
				return invalidInvocation("sftp accepts -r or --recursive only once")
			}
			called.Recursive = true
		case "--overwrite":
			if called.Overwrite {
				return invalidInvocation("sftp accepts --overwrite only once")
			}
			called.Overwrite = true
		case "--skip-existing":
			if called.SkipExisting {
				return invalidInvocation("sftp accepts --skip-existing only once")
			}
			called.SkipExisting = true
		case "--dry-run":
			if called.DryRun {
				return invalidInvocation("sftp accepts --dry-run only once")
			}
			called.DryRun = true
		case "--json":
			if called.JSON {
				return invalidInvocation("sftp accepts --json only once")
			}
			called.JSON = true
		case "-y", "--yes":
			if called.Yes {
				return invalidInvocation("sftp accepts -y or --yes only once")
			}
			called.Yes = true
		case "-j", "--jobs":
			if jobsSet {
				return invalidInvocation("sftp accepts -j or --jobs only once")
			}
			if index+1 >= len(args) {
				return invalidInvocation("sftp --jobs requires a number from 1 to 8")
			}
			index++
			jobs, err := strconv.Atoi(args[index])
			if err != nil || jobs < 1 || jobs > 8 {
				return invalidInvocation("sftp --jobs requires a number from 1 to 8")
			}
			called.Jobs = jobs
			jobsSet = true
		case "--split-size":
			if splitSizeSet {
				return invalidInvocation("sftp accepts --split-size only once")
			}
			if index+1 >= len(args) {
				return invalidInvocation("sftp --split-size requires a MiB value from 16 to 1024")
			}
			index++
			size, err := strconv.Atoi(args[index])
			if err != nil || size < 16 || size > 1024 {
				return invalidInvocation("sftp --split-size requires a MiB value from 16 to 1024")
			}
			called.SplitSizeMiB = size
			splitSizeSet = true
		case "--split-jobs":
			if splitJobsSet {
				return invalidInvocation("sftp accepts --split-jobs only once")
			}
			if index+1 >= len(args) {
				return invalidInvocation("sftp --split-jobs requires a number from 1 to 8")
			}
			index++
			jobs, err := strconv.Atoi(args[index])
			if err != nil || jobs < 1 || jobs > 8 {
				return invalidInvocation("sftp --split-jobs requires a number from 1 to 8")
			}
			called.SplitJobs = jobs
			splitJobsSet = true
		case "--chunk-size":
			if chunkSizeSet {
				return invalidInvocation("sftp accepts --chunk-size only once")
			}
			if index+1 >= len(args) {
				return invalidInvocation("sftp --chunk-size requires a MiB value from 8 to 4096")
			}
			index++
			size, err := strconv.Atoi(args[index])
			if err != nil || size < 8 || size > 4096 {
				return invalidInvocation("sftp --chunk-size requires a MiB value from 8 to 4096")
			}
			called.ChunkSizeMiB = size
			chunkSizeSet = true
		default:
			if value, found := strings.CutPrefix(argument, "--jobs="); found {
				if jobsSet {
					return invalidInvocation("sftp accepts -j or --jobs only once")
				}
				jobs, err := strconv.Atoi(value)
				if err != nil || jobs < 1 || jobs > 8 {
					return invalidInvocation("sftp --jobs requires a number from 1 to 8")
				}
				called.Jobs = jobs
				jobsSet = true
				continue
			}
			if strings.HasPrefix(argument, "-") {
				return invalidInvocation(fmt.Sprintf("unknown sftp option %q", argument))
			}
			positionals = append(positionals, argument)
		}
	}
	if len(positionals) != 3 || positionals[0] == "" || positionals[1] == "" || positionals[2] == "" {
		return invalidInvocation("sftp get and put require an alias, source, and destination")
	}
	if called.Overwrite && called.SkipExisting {
		return invalidInvocation("sftp cannot combine --overwrite and --skip-existing")
	}
	if called.Yes && !called.Overwrite {
		return invalidInvocation("sftp --yes requires --overwrite")
	}
	called.Alias, called.Source, called.Destination = positionals[0], positionals[1], positionals[2]
	return invocation{Kind: invocationSFTP, JSON: called.JSON, Yes: called.Yes, SFTP: &called}, nil
}

func parseSFTPSettingsInvocation(args []string) (invocation, error) {
	called := sftpInvocation{Action: sftpSettings}
	splitSizeSet := false
	splitJobsSet := false
	chunkSizeSet := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			if called.JSON {
				return invalidInvocation("sftp settings accepts --json only once")
			}
			called.JSON = true
		case "--split-size":
			if splitSizeSet || index+1 >= len(args) {
				return invalidInvocation("sftp settings --split-size requires one MiB value from 16 to 1024")
			}
			index++
			size, err := strconv.Atoi(args[index])
			if err != nil || size < 16 || size > 1024 {
				return invalidInvocation("sftp settings --split-size requires one MiB value from 16 to 1024")
			}
			called.SplitSizeMiB = size
			splitSizeSet = true
		case "--split-jobs":
			if splitJobsSet || index+1 >= len(args) {
				return invalidInvocation("sftp settings --split-jobs requires one number from 1 to 8")
			}
			index++
			jobs, err := strconv.Atoi(args[index])
			if err != nil || jobs < 1 || jobs > 8 {
				return invalidInvocation("sftp settings --split-jobs requires one number from 1 to 8")
			}
			called.SplitJobs = jobs
			splitJobsSet = true
		case "--chunk-size":
			if chunkSizeSet || index+1 >= len(args) {
				return invalidInvocation("sftp settings --chunk-size requires one MiB value from 8 to 4096")
			}
			index++
			size, err := strconv.Atoi(args[index])
			if err != nil || size < 8 || size > 4096 {
				return invalidInvocation("sftp settings --chunk-size requires one MiB value from 8 to 4096")
			}
			called.ChunkSizeMiB = size
			chunkSizeSet = true
		default:
			return invalidInvocation(fmt.Sprintf("unknown sftp settings option %q", args[index]))
		}
	}
	return invocation{Kind: invocationSFTP, JSON: called.JSON, SFTP: &called}, nil
}

func parseConfirmationOnlyInvocation(kind invocationKind, command string, args []string) (invocation, error) {
	called := invocation{Kind: kind}
	for _, argument := range args {
		switch argument {
		case "-y", "--yes":
			if called.Yes {
				return invalidInvocation(command + " accepts -y or --yes only once")
			}
			called.Yes = true
		default:
			return invalidInvocation(command + " only accepts -y or --yes")
		}
	}
	return called, nil
}

func helpInvocation(topic string) invocation {
	return invocation{Kind: invocationHelp, HelpTopic: topic}
}

func helpRequested(args []string) bool {
	return len(args) > 0 && isHelpFlag(args[0])
}

func isHelpFlag(argument string) bool {
	return argument == "-h" || argument == "--help"
}

func parseInfoInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandInfo)), nil
	}
	if len(args) < 1 || len(args) > 2 || args[0] == "" || args[0][0] == '-' {
		return invalidInvocation("info requires one alias followed by optional --json")
	}
	called := invocation{Kind: invocationInfo, Args: []string{args[0]}}
	if len(args) == 2 {
		if args[1] != "--json" {
			return invalidInvocation("info only accepts --json after the alias")
		}
		called.JSON = true
	}
	return called, nil
}

func parseSyncInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandSync)), nil
	}
	if len(args) > 1 && validSyncAction(args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandSync) + " " + args[0]), nil
	}
	if len(args) == 0 {
		return newSyncInvocation(syncInvocation{Action: syncStatus}), nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return newSyncInvocation(syncInvocation{Action: syncStatus, JSON: true}), nil
	}

	action := args[0]
	rest := args[1:]
	switch action {
	case "setup":
		if len(rest) != 0 {
			return invalidInvocation("sync setup takes no flags")
		}
		return newSyncInvocation(syncInvocation{Action: syncSetup}), nil
	case "push", "pull":
		parsed := syncInvocation{Action: syncPush}
		if action == "pull" {
			parsed.Action = syncPull
		}
		for _, flag := range rest {
			switch flag {
			case "--force":
				if parsed.Force {
					return invalidInvocation("sync accepts --force only once")
				}
				parsed.Force = true
			case "--json":
				if parsed.JSON {
					return invalidInvocation("sync accepts --json only once")
				}
				parsed.JSON = true
			default:
				return invalidInvocation("sync " + action + " does not take " + flag)
			}
		}
		return newSyncInvocation(parsed), nil
	case "now":
		asJSON, err := parseOptionalJSON(rest, "sync now")
		if err != nil {
			return invocation{Kind: invocationInvalid}, err
		}
		return newSyncInvocation(syncInvocation{Action: syncNow, JSON: asJSON}), nil
	case "auto":
		if len(rest) < 1 || (rest[0] != "on" && rest[0] != "off") {
			return invalidInvocation("sync auto requires on or off")
		}
		asJSON, err := parseOptionalJSON(rest[1:], "sync auto")
		if err != nil {
			return invocation{Kind: invocationInvalid}, err
		}
		return newSyncInvocation(syncInvocation{
			Action: syncAuto, Enabled: rest[0] == "on", JSON: asJSON,
		}), nil
	default:
		return invalidInvocation(fmt.Sprintf("unknown sync action %q", action))
	}
}

func parseOptionalJSON(args []string, command string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, nil
	}
	return false, fmt.Errorf("usage: %s only accepts one --json", command)
}

func newSyncInvocation(sync syncInvocation) invocation {
	return invocation{Kind: invocationSync, JSON: sync.JSON, Sync: &sync}
}

// parseSSHInvocation は alias を ssh namespace の内側だけで解釈する。
// transport や将来の top-level command と同名でも、alias の意味は変わらない。
func parseSSHInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandSsh)), nil
	}
	if len(args) == 0 {
		return invocation{Kind: invocationChoose}, nil
	}
	if len(args) == 1 {
		switch args[0] {
		case "--list":
			return invocation{Kind: invocationList}, nil
		}
		if args[0] == "" || args[0][0] == '-' {
			return invalidInvocation("ssh requires an alias")
		}
		return invocation{Kind: invocationConnect, Args: copyInvocationArgs(args)}, nil
	}
	if args[0] == "" || args[0][0] == '-' {
		return invalidInvocation("non-interactive SSH requires an alias before --non-interactive")
	}
	if len(args) < 4 || args[1] != "--non-interactive" || args[2] != "--" {
		return invalidInvocation("non-interactive SSH requires an alias, --non-interactive, --, and a command")
	}
	return invocation{Kind: invocationRun, Args: append([]string{args[0]}, args[3:]...)}, nil
}

// parseEngineFlags は `sshc engine` のオプションを解析する。
func parseEngineFlags(args []string) (invocation, error) {
	called := invocation{Kind: invocationEngine}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--replace":
			called.Replace = true
		case "--port":
			index++
			if index >= len(args) {
				return invalidInvocation("--port takes a number")
			}
			port, err := strconv.Atoi(args[index])
			// 無効な範囲と bind エラーを区別するため、ここで範囲を検証する。
			if err != nil || port < 1024 || port > 65535 {
				return invalidInvocation("--port takes a number between 1024 and 65535")
			}
			called.Port = port
		default:
			return invalidInvocation("engine does not take " + args[index])
		}
	}
	return called, nil
}

func noArguments(kind invocationKind, command string, args []string) (invocation, error) {
	if len(args) != 0 {
		return invalidInvocation(fmt.Sprintf("%s takes no arguments", command))
	}
	return invocation{Kind: kind}, nil
}

func copyInvocationArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	return append([]string(nil), args...)
}

func invalidInvocation(reason string) (invocation, error) {
	return invocation{Kind: invocationInvalid}, fmt.Errorf("usage: %s", reason)
}

// usage は生成済みのCLI契約を出力する。
func usage(out io.Writer) {
	fmt.Fprint(out, generatedGlobalHelp)
}

func usageFor(out io.Writer, topic string) {
	if topic == "" {
		usage(out)
		return
	}
	fmt.Fprint(out, generatedCLIHelp[topic])
}

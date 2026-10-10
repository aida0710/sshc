package main

import (
	sftpcore "sshc/internal/sftp"
)

type sftpAction uint8

const (
	sftpInvalid sftpAction = iota
	sftpGet
	sftpPut
	sftpSettings
)

type sftpInvocation struct {
	Action            sftpAction
	Alias             string
	Source            string
	Destination       string
	Recursive         bool
	Overwrite         bool
	SkipExisting      bool
	DryRun            bool
	JSON              bool
	Yes               bool
	Jobs              int
	SplitSizeMiB      int
	SplitJobs         int
	ChunkSizeMiB      int
	SpeedLimitKiB     *int
	ReconnectAttempts *int
	// Snapshot of persisted exclusions for this recursive invocation.
	ExcludePatterns []string
	MaxDepth        int
	MaxEntries      int
	MaxTotalMiB     int64
}

// sftpDefaultJobs は、--jobs を指定しないときに同時に転送するファイルの数である。
const sftpDefaultJobs = 1

var (
	sftpJobsBounds       = integerBounds{minimum: 1, maximum: sftpcore.MaxTransferConcurrency, kind: "a number"}
	sftpSplitSizeBounds  = integerBounds{minimum: sftpCLIMinSplitSizeMiB, maximum: sftpCLIMaxSplitSizeMiB, kind: "a MiB value"}
	sftpSplitJobsBounds  = integerBounds{minimum: 1, maximum: sftpcore.MaxLargeFileParallelism, kind: "a number"}
	sftpChunkSizeBounds  = integerBounds{minimum: sftpCLIMinChunkSizeMiB, maximum: sftpCLIMaxChunkSizeMiB, kind: "a MiB value"}
	sftpMaxDepthBounds   = integerBounds{minimum: 1, maximum: sftpCLIMaxRecursiveDepth, kind: "a number"}
	sftpMaxEntriesBounds = integerBounds{minimum: 1, maximum: sftpCLIMaxRecursiveEntries, kind: "a number"}
	sftpMaxTotalBounds   = integerBounds{minimum: 1, maximum: int(sftpCLIMaxRecursiveMiB), kind: "a MiB value"}
)

// sftpSplitOptions は、get／put と settings が同じ意味で受け取る分割転送のオプションである。
var sftpSplitOptions = []commandOption{
	valueOption("--split-size"), valueOption("--split-jobs"), valueOption("--chunk-size"),
}

var sftpTransferOptions = commandOptions{command: "sftp", options: append([]commandOption{
	switchOption("--recursive", "-r"), switchOption("--overwrite"), switchOption("--skip-existing"),
	switchOption("--dry-run"), jsonOption, yesOption, valueOption("--jobs", "-j"),
	valueOption("--max-depth"), valueOption("--max-entries"), valueOption("--max-total-size"),
}, sftpSplitOptions...)}

var sftpSettingsOptions = commandOptions{
	command: "sftp settings", options: append([]commandOption{jsonOption, valueOption("--speed-limit"), valueOption("--reconnect-attempts")}, sftpSplitOptions...),
}

// sftpRecursiveLimitOptions は、再帰の get だけが受け取る安全の上限である。
var sftpRecursiveLimitOptions = []string{"--max-depth", "--max-entries", "--max-total-size"}

func parseSFTPInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandSftp)), nil
	}
	if len(args) > 1 && validCLIAction("sftp", args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandSftp) + " " + args[0]), nil
	}
	if len(args) == 0 || !validCLIAction("sftp", args[0]) {
		return invalidInvocation(missingActionMessage("sftp"))
	}
	if args[0] == "settings" {
		return parseSFTPSettingsInvocation(args[1:])
	}
	arguments, err := sftpTransferOptions.parse(args[1:])
	if err != nil {
		return invalidInvocation(err.Error())
	}
	called, err := readSFTPTransferOptions(arguments)
	if err != nil {
		return invalidInvocation(err.Error())
	}
	called.Action = sftpGet
	if args[0] == "put" {
		called.Action = sftpPut
	}
	positionals := arguments.positionals
	if len(positionals) != 3 || positionals[0] == "" || positionals[1] == "" || positionals[2] == "" {
		return invalidInvocation("sftp get and put require an alias, source, and destination")
	}
	if called.Overwrite && called.SkipExisting {
		return invalidInvocation("sftp cannot combine --overwrite and --skip-existing")
	}
	if called.Yes && !called.Overwrite {
		return invalidInvocation("sftp --yes requires --overwrite")
	}
	if arguments.hasAny(sftpRecursiveLimitOptions...) {
		if called.Action != sftpGet {
			return invalidInvocation("sftp recursive safety limits are available only for get")
		}
		if !called.Recursive {
			return invalidInvocation("sftp recursive safety limits require --recursive")
		}
	}
	called.Alias, called.Source, called.Destination = positionals[0], positionals[1], positionals[2]
	return invocation{Kind: invocationSFTP, JSON: called.JSON, Yes: called.Yes, SFTP: &called}, nil
}

func readSFTPTransferOptions(arguments commandArguments) (sftpInvocation, error) {
	called := sftpInvocation{
		Recursive: arguments.has("--recursive"), Overwrite: arguments.has("--overwrite"),
		SkipExisting: arguments.has("--skip-existing"), DryRun: arguments.has("--dry-run"),
		JSON: arguments.has("--json"), Yes: arguments.has("--yes"),
	}
	var err error
	if called.Jobs, err = arguments.integer("--jobs", sftpJobsBounds, sftpDefaultJobs); err != nil {
		return sftpInvocation{}, err
	}
	if err := readSFTPSplitOptions(arguments, &called); err != nil {
		return sftpInvocation{}, err
	}
	if called.MaxDepth, err = arguments.integer("--max-depth", sftpMaxDepthBounds, sftpCLIDefaultRecursiveDepth); err != nil {
		return sftpInvocation{}, err
	}
	if called.MaxEntries, err = arguments.integer("--max-entries", sftpMaxEntriesBounds, sftpCLIDefaultRecursiveEntries); err != nil {
		return sftpInvocation{}, err
	}
	maxTotalMiB, err := arguments.integer("--max-total-size", sftpMaxTotalBounds, int(sftpCLIDefaultRecursiveBytes>>20))
	if err != nil {
		return sftpInvocation{}, err
	}
	called.MaxTotalMiB = int64(maxTotalMiB)
	return called, nil
}

// readSFTPSplitOptions は分割転送のオプションを読む。指定されなかった値は 0 のままにし、
// engine の設定をそのまま使わせる。
func readSFTPSplitOptions(arguments commandArguments, called *sftpInvocation) error {
	var err error
	if called.SplitSizeMiB, err = arguments.integer("--split-size", sftpSplitSizeBounds, 0); err != nil {
		return err
	}
	if called.SplitJobs, err = arguments.integer("--split-jobs", sftpSplitJobsBounds, 0); err != nil {
		return err
	}
	called.ChunkSizeMiB, err = arguments.integer("--chunk-size", sftpChunkSizeBounds, 0)
	return err
}

func parseSFTPSettingsInvocation(args []string) (invocation, error) {
	arguments, err := sftpSettingsOptions.parseWithoutPositionals(args)
	if err != nil {
		return invalidInvocation(err.Error())
	}
	called := sftpInvocation{Action: sftpSettings, JSON: arguments.has("--json")}
	if err := readSFTPSplitOptions(arguments, &called); err != nil {
		return invalidInvocation(err.Error())
	}
	if err := readSFTPRecoverySettings(arguments, &called); err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: invocationSFTP, JSON: called.JSON, SFTP: &called}, nil
}

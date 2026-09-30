package main

import "fmt"

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
		arguments, err := syncTransferOptions(action).parseWithoutPositionals(rest)
		if err != nil {
			return invalidInvocation(err.Error())
		}
		parsed := syncInvocation{Action: syncPush, Force: arguments.has("--force"), JSON: arguments.has("--json")}
		if action == "pull" {
			parsed.Action = syncPull
		}
		return newSyncInvocation(parsed), nil
	case "now":
		asJSON, err := parseOptionalJSON("sync now", rest)
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return newSyncInvocation(syncInvocation{Action: syncNow, JSON: asJSON}), nil
	case "auto":
		if len(rest) < 1 || (rest[0] != "on" && rest[0] != "off") {
			return invalidInvocation("sync auto requires on or off")
		}
		asJSON, err := parseOptionalJSON("sync auto", rest[1:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return newSyncInvocation(syncInvocation{
			Action: syncAuto, Enabled: rest[0] == "on", JSON: asJSON,
		}), nil
	default:
		return invalidInvocation(fmt.Sprintf("unknown sync action %q", action))
	}
}

// syncTransferOptions は、sync push と sync pull が受け取るオプションである。
func syncTransferOptions(action string) commandOptions {
	return commandOptions{command: "sync " + action, options: []commandOption{switchOption("--force"), jsonOption}}
}

func newSyncInvocation(sync syncInvocation) invocation {
	return invocation{Kind: invocationSync, JSON: sync.JSON, Sync: &sync}
}

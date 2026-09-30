package main

import (
	"fmt"
	"sort"
	"strings"
)

// cliActions は、コマンドごとのアクション名。clispec の Actions から生成したヘルプの
// 項目（"<command> <action>"）から集めるので、clispec にアクションを足せば、
// 補完と引数の誤りの文にもそのまま届く。
var cliActions = collectCLIActions(generatedCLIHelp)

func collectCLIActions(help map[string]string) map[string][]string {
	actions := map[string][]string{}
	for topic := range help {
		command, action, isAction := strings.Cut(topic, " ")
		if isAction {
			actions[command] = append(actions[command], action)
		}
	}
	for _, names := range actions {
		sort.Strings(names)
	}
	return actions
}

// missingActionMessage は、アクションを付けずに呼んだときの誤りの文を作る。
func missingActionMessage(command string) string {
	names := cliActions[command]
	switch len(names) {
	case 0:
		return command + " requires an action"
	case 1:
		return fmt.Sprintf("%s requires %s", command, names[0])
	default:
		return fmt.Sprintf("%s requires %s, or %s", command,
			strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
	}
}

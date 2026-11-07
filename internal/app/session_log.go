package app

import (
	"errors"
	"path/filepath"

	"sshc/internal/application"
	"sshc/internal/sessionlog"
	"sshc/internal/terminal"
)

// localShellLogFileName は、ローカルシェルのログのファイル名の頭である。照らす名前（sessionlog.LocalShellName）の
// @ は、ファイル名の頭には置かない。
const localShellLogFileName = "local"

// openSessionLog は、開くセッションのログを、設定が保存すると言うときに開く（internal/sessionlog）。設定は
// セッションを開くたびに読むので、変えると次に開くセッションから効く。ログを保存できないマシン（Android）
// では何もしない。
func openSessionLog(config *application.Service) func(terminal.LogTarget) (terminal.OutputLog, error) {
	return func(target terminal.LogTarget) (terminal.OutputLog, error) {
		name, fileName := target.Alias, target.Alias
		if target.Kind == terminal.KindShell {
			name, fileName = sessionlog.LocalShellName, localShellLogFileName
		}
		if !config.SessionLogKeeps(name) {
			return nil, nil
		}
		directory, err := config.SessionLogDirectory()
		if errors.Is(err, sessionlog.ErrUnsupported) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		log, err := sessionlog.Open(filepath.Join(directory, sessionlog.FileName(fileName, target.ID, target.Started)), target.Size)
		if err != nil {
			return nil, err
		}
		return log, nil
	}
}

package application

import (
	"os"
	goruntime "runtime"

	"sshc/internal/sessionlog"
)

// SessionLogDirectory は、このマシンがセッションのログを置くフォルダを返す。マシンの設定に無ければ OS の
// 決まった場所。ログを保存できないマシン（Android）では sessionlog.ErrUnsupported。同期するフォルダ（~/.ssh）の
// 下は sessionlog.ErrSynchronizedDirectory で断る。設定はほかのマシンの画面からも変えられ、保存のときは
// このマシンのパスの形が分からないので、使うここで確かめる。
func (s *Service) SessionLogDirectory() (string, error) {
	directory, err := sessionlog.Directory(s.ThisMachineSettings().SessionLogDirectory, goruntime.GOOS, s.workspace.Home(), os.Getenv)
	if err != nil {
		return "", err
	}
	if err := sessionlog.RefuseSynchronized(directory, s.workspace.Root(), goruntime.GOOS); err != nil {
		return "", err
	}
	return directory, nil
}

// DefaultSessionLogDirectory は、このマシンの OS の決まったログの場所である（MachineFacts に書く）。保存
// できないマシンでは空。
func (s *Service) DefaultSessionLogDirectory() string {
	directory, err := sessionlog.DefaultDirectory(goruntime.GOOS, s.workspace.Home(), os.Getenv)
	if err != nil {
		return ""
	}
	return directory
}

// SessionLogKeeps は、name のセッションのログを保存するかを、全体の設定から返す。name は接続名（保存して
// いない接続先は user@host）、ローカルシェルは sessionlog.LocalShellName である。
func (s *Service) SessionLogKeeps(name string) bool {
	settings := s.globalTerminalSettings()
	if !settings.SessionLog {
		return false
	}
	rules, err := sessionlog.CompileRules(settings.SessionLogRules)
	return err == nil && rules.Keeps(name)
}

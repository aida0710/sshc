package application

import (
	"errors"

	"sshc/internal/platform"
	"sshc/internal/terminal"
)

// 開始位置を断る理由。
var (
	// ErrStartDirectoryMissing は、そこに無いディレクトリを断る。
	ErrStartDirectoryMissing = errors.New("that directory does not exist")
	// ErrStartDirectoryNotADirectory は、ファイルを指した指定を断る。
	ErrStartDirectoryNotADirectory = errors.New("that path is not a directory")
)

// terminalPresetSettings は、このマシンが使うプリセットのターミナルの設定を、書かれていない節を
// 空として返す。
func (s *Service) terminalPresetSettings() EmbeddedTerminal {
	if terminal := s.EffectiveSettings().Preset.Terminal; terminal != nil {
		return *terminal
	}
	return EmbeddedTerminal{}
}

// globalTerminalSettings は、全体の設定のうちターミナルのものを、書かれていない節を空として返す。
func (s *Service) globalTerminalSettings() GlobalTerminalSettings {
	if terminal := s.EffectiveSettings().Global.Terminal; terminal != nil {
		return *terminal
	}
	return GlobalTerminalSettings{}
}

// TerminalVerbosity は、接続の途中経過をどこまでターミナルへ書くかを返す。
func (s *Service) TerminalVerbosity() int {
	return s.globalTerminalSettings().Verbosity
}

// TerminalReconnects は、繋ぎ直しを何回まで試みるかを返す。
func (s *Service) TerminalReconnects() int {
	reconnect := s.globalTerminalSettings().Reconnect
	if reconnect == nil {
		return terminal.MaxReconnects
	}
	return terminal.NormaliseReconnects(*reconnect)
}

// TerminalLimits は、埋め込みターミナルが開くたびに読む上限を返す。
func (s *Service) TerminalLimits() terminal.Limits {
	settings := s.terminalPresetSettings()
	return terminal.Limits{MaxSessions: settings.MaxSessions, Scrollback: settings.ScrollbackBytes}.Normalise()
}

// TerminalStartDirectory は、ローカルシェルを始める絶対パスを返す。プリセットの指定がこの
// マシンに無いディレクトリ（別の OS のパスなど）なら、ホームディレクトリから始める。
func (s *Service) TerminalStartDirectory() string {
	home := s.workspace.Home()
	resolved, err := platform.ResolveUnderHome(s.terminalPresetSettings().StartDirectory, home)
	if err != nil || resolved == "" {
		return home
	}
	if err := s.directoryExists(resolved); err != nil {
		return home
	}
	return resolved
}

// TerminalLocalShellProfile returns the stable local profile ID of the preset this machine uses.
func (s *Service) TerminalLocalShellProfile() string {
	return s.terminalPresetSettings().LocalShellProfile
}

// checkStartDirectoryChange は、プリセットの開始ディレクトリを変えるとき、このマシンに入れる
// ディレクトリかを確かめる。変えないときは確かめない。開始ディレクトリは別の OS のマシンや後から
// 消したディレクトリを指していることがあり、ほかの項目の保存まで止めないためである。
func (s *Service) checkStartDirectoryChange(previous, next SettingsPreset) error {
	before, after := "", ""
	if previous.Terminal != nil {
		before = previous.Terminal.StartDirectory
	}
	if next.Terminal != nil {
		after = next.Terminal.StartDirectory
	}
	if before == after {
		return nil
	}
	resolved, err := platform.ResolveUnderHome(after, s.workspace.Home())
	if err != nil {
		return err
	}
	if resolved == "" {
		return nil
	}
	return s.directoryExists(resolved)
}

// directoryExists は、そこに入れるディレクトリがあることを確かめる。
func (s *Service) directoryExists(path string) error {
	fileSystem := s.workspace.FileSystem()
	target, err := fileSystem.EvalSymlinks(path)
	if err != nil {
		return ErrStartDirectoryMissing
	}
	info, err := fileSystem.Lstat(target)
	if err != nil {
		return ErrStartDirectoryMissing
	}
	if !info.IsDir() {
		return ErrStartDirectoryNotADirectory
	}
	return nil
}

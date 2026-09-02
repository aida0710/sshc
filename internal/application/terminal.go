package application

import (
	"errors"

	"sshc/internal/platform"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

// 開始位置を断る理由。
var (
	// ErrStartDirectoryMissing は、そこに無いディレクトリを断る。
	ErrStartDirectoryMissing = errors.New("that directory does not exist")
	// ErrStartDirectoryNotADirectory は、ファイルを指した指定を断る。
	ErrStartDirectoryNotADirectory = errors.New("that path is not a directory")
)

// TerminalSettings は、埋め込みターミナルのうち画面から変えられるものである。
type TerminalSettings struct {
	// StartDirectory は書かれた表記のまま。`~/work` は `~/work` である。
	StartDirectory string
	// MaxSessions と ScrollbackBytes は、範囲の外なら書き込みで拒否される。
	MaxSessions     int
	ScrollbackBytes int
	// FontSize は画面が字を描く大きさ。この engine は読むだけで、使わない。
	FontSize int
	// Verbosity は、接続の途中経過をどこまで端末へ書くかである。0 は無言。
	Verbosity int
	// Reconnect は、輸送が落ちたときに繋ぎ直しを試みる回数である。
	Reconnect *int
	// nil は既定の on、false は明示的に止めた値である。
	CopyOnSelect           *bool
	RightClickPaste        *bool
	BrowserScrollbackLines int
	OSC52                  bool
	JISYenBackslash        bool
	LocalShellProfile      string
	// Appearance は、どの接続にも選ばれていないときの見た目である。
	Appearance TerminalAppearance
}

// TerminalSettings は、保存されている値をそのまま返す。
func (s *Service) TerminalSettings() TerminalSettings {
	stored, _, err := s.metadata.Load()
	if err != nil || stored.EmbeddedTerminal == nil {
		return TerminalSettings{}
	}
	return TerminalSettings{
		StartDirectory:         stored.EmbeddedTerminal.StartDirectory,
		MaxSessions:            stored.EmbeddedTerminal.MaxSessions,
		ScrollbackBytes:        stored.EmbeddedTerminal.ScrollbackBytes,
		FontSize:               stored.EmbeddedTerminal.FontSize,
		Verbosity:              stored.EmbeddedTerminal.Verbosity,
		Reconnect:              stored.EmbeddedTerminal.Reconnect,
		CopyOnSelect:           stored.EmbeddedTerminal.CopyOnSelect,
		RightClickPaste:        stored.EmbeddedTerminal.RightClickPaste,
		BrowserScrollbackLines: stored.EmbeddedTerminal.BrowserScrollbackLines,
		OSC52:                  stored.EmbeddedTerminal.OSC52,
		JISYenBackslash:        stored.EmbeddedTerminal.JISYenBackslash,
		LocalShellProfile:      stored.EmbeddedTerminal.LocalShellProfile,
		Appearance:             appearanceOf(stored.EmbeddedTerminal.Appearance),
	}
}

// appearanceOf は、書かれていない節を空として読む。
func appearanceOf(stored *TerminalAppearance) TerminalAppearance {
	if stored == nil {
		return TerminalAppearance{}
	}
	return *stored
}

// SetTerminalSettings は、節をまるごと置き換える。
func (s *Service) SetTerminalSettings(settings TerminalSettings) (SaveResult, error) {
	resolved, err := platform.ResolveUnderHome(settings.StartDirectory, s.workspace.Home())
	if err != nil {
		return SaveResult{}, err
	}
	if resolved != "" {
		if err := s.directoryExists(resolved); err != nil {
			return SaveResult{}, err
		}
	}

	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if settings == (TerminalSettings{}) {
		// 何も設定されていないなら節ごと消す。空の節を残さない。
		stored.EmbeddedTerminal = nil
	} else {
		stored.EmbeddedTerminal = &EmbeddedTerminal{
			MaxSessions:            settings.MaxSessions,
			ScrollbackBytes:        settings.ScrollbackBytes,
			FontSize:               settings.FontSize,
			Verbosity:              settings.Verbosity,
			Reconnect:              settings.Reconnect,
			StartDirectory:         settings.StartDirectory,
			CopyOnSelect:           settings.CopyOnSelect,
			RightClickPaste:        settings.RightClickPaste,
			BrowserScrollbackLines: settings.BrowserScrollbackLines,
			OSC52:                  settings.OSC52,
			JISYenBackslash:        settings.JISYenBackslash,
			LocalShellProfile:      settings.LocalShellProfile,
			// 空の節は書かない。 残せば、次に読む者は何か選ばれていると思う。
			Appearance: storedAppearance(settings.Appearance),
		}
	}

	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "terminal.settings",
		Changes:   []storage.Change{change},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
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

// TerminalStartDirectory は、ローカルシェルを始める絶対パスを返す。
func (s *Service) TerminalStartDirectory() string {
	home := s.workspace.Home()
	resolved, err := platform.ResolveUnderHome(s.TerminalSettings().StartDirectory, home)
	if err != nil || resolved == "" {
		return home
	}
	if err := s.directoryExists(resolved); err != nil {
		return home
	}
	return resolved
}

// TerminalLocalShellProfile returns the persisted stable local profile ID.
func (s *Service) TerminalLocalShellProfile() string {
	return s.TerminalSettings().LocalShellProfile
}

// storedAppearance は、何も選ばれていない見た目を「書かれていない」に潰す。
func storedAppearance(chosen TerminalAppearance) *TerminalAppearance {
	if chosen.Empty() {
		return nil
	}
	return &chosen
}

// FileTransferSettings は、保存されている転送キューの設定をそのまま返す。
func (s *Service) FileTransferSettings() FileTransferSettings {
	stored, _, err := s.metadata.Load()
	if err != nil || stored.FileTransfers == nil {
		return FileTransferSettings{}
	}
	return *stored.FileTransfers
}

// SetFileTransferSettings は、節をまるごと置き換える。
func (s *Service) SetFileTransferSettings(settings FileTransferSettings) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if settings == (FileTransferSettings{}) {
		// 既定のままなら節ごと消す。空の節を残さない。
		stored.FileTransfers = nil
	} else {
		stored.FileTransfers = &settings
	}
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "sftp.transferSettings",
		Changes:   []storage.Change{change},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}

// EngineSettings は、保存されている engine の設定をそのまま返す。
func (s *Service) EngineSettings() EngineSettings {
	stored, _, err := s.metadata.Load()
	if err != nil || stored.Engine == nil {
		return EngineSettings{}
	}
	return *stored.Engine
}

// SetEngineSettings は、節をまるごと置き換える。
func (s *Service) SetEngineSettings(settings EngineSettings) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if settings == (EngineSettings{}) {
		// 何も設定されていないなら節ごと消す。空の節を残さない。
		stored.Engine = nil
	} else {
		stored.Engine = &settings
	}
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "engine.settings",
		Changes:   []storage.Change{change},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}

// TerminalReconnects は、繋ぎ直しを何回まで試みるかを返す。
func (s *Service) TerminalReconnects() int {
	settings := s.TerminalSettings()
	if settings.Reconnect == nil {
		return terminal.MaxReconnects
	}
	return terminal.NormaliseReconnects(*settings.Reconnect)
}

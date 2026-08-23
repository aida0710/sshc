package application

import (
	"errors"

	"sshc/internal/platform"
	"sshc/internal/storage"
)

// 開始位置を断る理由。
var (
	// ErrStartDirectoryMissing は、そこに無いディレクトリを断る。
	//
	// **保存のときに断る。** 通らないものを黙って受け取ると、次に端末を開いた
	// ときに初めて分かる——設定画面と、失敗が現れる場所が離れる。
	ErrStartDirectoryMissing = errors.New("that directory does not exist")
	// ErrStartDirectoryNotADirectory は、ファイルを指した指定を断る。
	ErrStartDirectoryNotADirectory = errors.New("that path is not a directory")
)

// TerminalSettings は、埋め込みターミナルのうち画面から変えられるものである。
//
// **0 と空は「設定されていない」である。** 「既定と同じ値」ではない——
// 区別が要るのは、既定と同じ値を書き戻すと、それが metadata に焼き付くから
// である。焼き付けば、既定を変えた日にその人だけが黙って取り残される。
type TerminalSettings struct {
	// StartDirectory は書かれた綴りのまま。`~/work` は `~/work` である。
	StartDirectory string
	// MaxSessions と ScrollbackBytes は、範囲の外なら書き込みで拒否される。
	// 読み取り側（TerminalLimits）は範囲の外を既定へ戻す。
	MaxSessions     int
	ScrollbackBytes int
	// FontSize は画面が字を描く大きさ。この engine は読むだけで、使わない。
	FontSize int
	// nil は既定の on、false は明示的に止めた値である。
	CopyOnSelect    *bool
	RightClickPaste *bool
	// Appearance は、どの接続にも選ばれていないときの見た目である。
	//
	// **値で持つ。** この構造体は `== (TerminalSettings{})` で「何も設定されて
	// いない」を判定している。ポインタにすると、空を指すポインタがその判定を
	// すり抜け、空の節が metadata に残る。
	Appearance TerminalAppearance
}

// TerminalSettings は、保存されている値をそのまま返す。
//
// **正規化しない。** 画面はこれを編集するので、既定へ戻した値を見せると、
// 人が何も触っていないのに「既定を明示的に選んだ」状態が保存されてしまう。
func (s *Service) TerminalSettings() TerminalSettings {
	stored, _, err := s.metadata.Load()
	if err != nil || stored.EmbeddedTerminal == nil {
		return TerminalSettings{}
	}
	return TerminalSettings{
		StartDirectory:  stored.EmbeddedTerminal.StartDirectory,
		MaxSessions:     stored.EmbeddedTerminal.MaxSessions,
		ScrollbackBytes: stored.EmbeddedTerminal.ScrollbackBytes,
		FontSize:        stored.EmbeddedTerminal.FontSize,
		CopyOnSelect:    stored.EmbeddedTerminal.CopyOnSelect,
		RightClickPaste: stored.EmbeddedTerminal.RightClickPaste,
		Appearance:      appearanceOf(stored.EmbeddedTerminal.Appearance),
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
//
// **置き換えなのは、消せる必要があるからである。** 一度指定した人が既定へ
// 戻れなければ、設定は片道になる。空で送られた項目は、書かれていない状態へ戻る。
//
// 綴りはそのまま保存する。`~/work` は `~/work` のまま metadata に入る——
// home の綴りを焼き付けると、その設定は書いた機械でしか意味を持たない。
// 確かめるのは展開したあとの実体である。
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
		// 何も設定されていないなら節ごと消す。**空の節を残さない**——
		// 残せば、次に読む者は「何か書かれている」と思う。
		stored.EmbeddedTerminal = nil
	} else {
		stored.EmbeddedTerminal = &EmbeddedTerminal{
			MaxSessions:     settings.MaxSessions,
			ScrollbackBytes: settings.ScrollbackBytes,
			FontSize:        settings.FontSize,
			StartDirectory:  settings.StartDirectory,
			CopyOnSelect:    settings.CopyOnSelect,
			RightClickPaste: settings.RightClickPaste,
			// **空の節は書かない。** 残せば、次に読む者は何か選ばれていると思う。
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
//
// シンボリックリンクは先を見る。**人がそう書くのは普通のこと**であり、
// リンクだからという理由で断ると、断られた理由が分からない。
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
//
// **読むたびに metadata を見る。** 設定は動いている最中に変わりうるので、
// 起動時に一度だけ読むと、変えた人は次に端末を開いても前の場所に立つ。
//
// 読めない、書かれていない、あるいはもう無い場所を指しているときは home を
// 返す。**端末が開けなくなる方が悪い**——開始位置は、開けることより弱い要求である。
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

// storedAppearance は、何も選ばれていない見た目を「書かれていない」に潰す。
func storedAppearance(chosen TerminalAppearance) *TerminalAppearance {
	if chosen.Empty() {
		return nil
	}
	return &chosen
}

// EngineSettings は、保存されている engine の設定をそのまま返す。
//
// **正規化しない。** 画面はこれを編集するので、既定へ戻した値を見せると、人が
// 何も触っていないのに「既定を明示的に選んだ」状態が保存されてしまう。
func (s *Service) EngineSettings() EngineSettings {
	stored, _, err := s.metadata.Load()
	if err != nil || stored.Engine == nil {
		return EngineSettings{}
	}
	return *stored.Engine
}

// SetEngineSettings は、節をまるごと置き換える。
//
// **置き換えなのは、消せる必要があるからである。** 一度番号を決めた人が無作為へ
// 戻れなければ、戻る手段は metadata を手で書くことだけになる。
func (s *Service) SetEngineSettings(settings EngineSettings) (SaveResult, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	if settings == (EngineSettings{}) {
		// 何も設定されていないなら節ごと消す。**空の節を残さない。**
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

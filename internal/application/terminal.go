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

// SetTerminalStartDirectory は、ローカルシェルが始まる場所を書き戻す。
//
// **綴りはそのまま持つ。** `~/work` と書いたら `~/work` のまま保存する——
// home の綴りを焼き付けると、その設定は書いた機械でしか意味を持たない。
// 確かめるのは展開したあとの実体である。
func (s *Service) SetTerminalStartDirectory(directory string) (SaveResult, error) {
	resolved, err := platform.ResolveUnderHome(directory, s.workspace.Home())
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
	if stored.EmbeddedTerminal == nil {
		stored.EmbeddedTerminal = &EmbeddedTerminal{}
	}
	stored.EmbeddedTerminal.StartDirectory = directory

	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(stored, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "terminal.startDirectory",
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
	stored, _, err := s.metadata.Load()
	if err != nil {
		return home
	}
	resolved, err := platform.ResolveUnderHome(stored.TerminalStartDirectory(), home)
	if err != nil || resolved == "" {
		return home
	}
	if err := s.directoryExists(resolved); err != nil {
		return home
	}
	return resolved
}

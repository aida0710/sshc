package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// 名前を決める前に、中身を読みながら書き終える一時ファイル。
//
// WriteAtomicFile は中身をメモリに持ってから書く。背景画像のように 1 GiB まで
// 大きくなりうる中身は、読みながら置き場所と同じディレクトリの一時ファイルへ書き、
// 書き終えてから名前を決めて rename で公開する。名前をあとで決めるのは、空き容量と
// 同じ名前の有無を、中身を受け取り終えた時点で確かめるためである。

// ErrSourceUnreadable は、StageFile が書き写す元を読めなかったことを示す。元の
// エラーも errors.Is と errors.As で取り出せる。呼び出し側は、元が途中で切れた
// （送り手の問題）ことと、書けなかった（このマシンの問題）ことを分けて伝える。
var ErrSourceUnreadable = errors.New("the contents to stage could not be read")

// StageRequest は、StageFile が作る一時ファイルの置き場所と中身である。
type StageRequest struct {
	// Directory は、一時ファイルを作り、Publish で公開するディレクトリである。
	Directory string
	// Prefix は一時ファイルの名前の先頭である。IsTemporaryName が認める ".sshc-" で
	// 始めれば、書いている途中のファイルやクラッシュで残ったファイルが、ディレクトリを
	// 走査する側（一覧、同期）に紛れ込まない。
	Prefix     string
	Permission fs.FileMode
	Source     io.Reader
	// Maximum は中身の上限である。Source がこれを超えたら ErrFileTooLarge を返し、
	// 何も残さない。
	Maximum int64
}

// StagedFile は、StageFile が書き終えてディスクへフラッシュした、まだ名前の無い
// 一時ファイルである。呼び出し側は Publish で公開するか、Discard で捨てる。
// 置き場所のディレクトリを開いたまま持つので、公開しない場合も Discard を呼ぶ。
type StagedFile struct {
	directory string
	size      int64
	temporary *stagedTemporary
	finished  bool
}

// Size は、書いた中身のバイト数である。
func (f *StagedFile) Size() int64 { return f.size }

// Publish は、一時ファイルを path へ 1 回の rename で置き、ディレクトリの変更を
// ディスクへ流す。path は StageFile に渡したディレクトリの直下でなければならない。
// path に既にファイルがあれば置き換える。
func (f *StagedFile) Publish(path string) error {
	if f.finished {
		return os.ErrClosed
	}
	cleaned := filepath.Clean(path)
	if filepath.Dir(cleaned) != f.directory {
		return os.ErrInvalid
	}
	if err := f.temporary.rename(filepath.Base(cleaned)); err != nil {
		return err
	}
	// rename が通った時点で一時ファイルは公開済みの名前になっている。このあと
	// Discard が呼ばれても、公開したファイルを消さない。
	f.finished = true
	defer f.temporary.release()
	return f.temporary.syncRename()
}

// Discard は、公開していない一時ファイルを消し、開いているディレクトリを閉じる。
// Publish のあとや 2 回目の呼び出しでは何もしない。
func (f *StagedFile) Discard() {
	if f.finished {
		return
	}
	f.finished = true
	f.temporary.remove()
	f.temporary.release()
}

// copyAndFlush は、request.Source を file へ request.Maximum バイトまで書き、
// 権限を与えてディスクへフラッシュする。書いたバイト数を返す。
//
// 1 バイト余分に読む。ちょうど上限で切ると、超えていることとちょうど収まって
// いることが見分けられない。
func copyAndFlush(file *os.File, request StageRequest) (int64, error) {
	if err := file.Chmod(request.Permission); err != nil {
		return 0, err
	}
	source := &sourceReader{reader: request.Source}
	written, err := io.Copy(file, io.LimitReader(source, request.Maximum+1))
	if source.err != nil {
		return 0, fmt.Errorf("%w: %w", ErrSourceUnreadable, source.err)
	}
	if err != nil {
		return 0, err
	}
	if written > request.Maximum {
		return 0, ErrFileTooLarge
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}
	return written, nil
}

// sourceReader は、元の読み取りで起きたエラーを覚えておく。io.Copy は読み取りと
// 書き込みのどちらで失敗したかを区別しないので、その区別をここで残す。
type sourceReader struct {
	reader io.Reader
	err    error
}

func (r *sourceReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return count, err
}

package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 名前を決める前に、中身を読みながら書き終える一時ファイル。
//
// WriteAtomicFileは中身をメモリに持ってから書く。背景画像のように1 GiBまで大きくなり
// うる中身は、読みながら置き場所と同じディレクトリの一時ファイルへ書き、書き終えてから
// 名前を決めてrenameで公開する。名前をあとで決めるのは、空き容量と同じ名前の有無を、
// 中身を受け取り終えた時点で確かめるためである。WriteAtomicFileのOSごとの実装も、
// この手順（stageFileNative）で書く。

// ErrSourceUnreadableは、StageFileが書き写す元を読めなかったことを示す。元のエラーも
// errors.Isとerrors.Asで取り出せる。呼び出し側は、元が途中で切れた（送り手の問題）ことと、
// 書けなかった（このマシンの問題）ことを分けて伝える。
var ErrSourceUnreadable = errors.New("the contents to stage could not be read")

// StageRequestは、StageFileが作る一時ファイルの置き場所と中身である。
type StageRequest struct {
	// Directoryは、一時ファイルを作り、Publishで公開するディレクトリである。
	Directory string
	// Prefixは一時ファイルの名前の先頭である。IsTemporaryNameが認める".sshc-"で始めれば、
	// 書いている途中のファイルやクラッシュで残ったファイルが、ディレクトリを走査する側
	// （一覧、同期）に紛れ込まない。
	Prefix     string
	Permission fs.FileMode
	Source     io.Reader
	// Maximumは中身の上限である。Sourceがこれを超えたらErrFileTooLargeを返し、何も残さない。
	Maximum int64
}

// StagedFileは、StageFileが書き終えてディスクへフラッシュした、まだ名前の無い一時ファイル
// である。呼び出し側はPublishで公開するか、Discardで捨てる。置き場所のディレクトリを
// 開いたまま持つので、公開しない場合もDiscardを呼ぶ。
type StagedFile struct {
	directory string
	size      int64
	temporary *stagedTemporary
	finished  bool
}

// Sizeは、書いた中身のバイト数である。
func (f *StagedFile) Size() int64 { return f.size }

// Publishは、一時ファイルをpathへ1回のrenameで置き、その置き換えをディスクへ流す。pathは
// StageFileに渡したディレクトリの直下でなければならない。pathに既にファイルがあれば
// 置き換える。
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
	// renameが通った時点で一時ファイルは公開済みの名前になっている。このあとDiscardが
	// 呼ばれても、公開したファイルを消さない。
	f.finished = true
	defer f.temporary.release()
	return f.temporary.finishPublish()
}

// Discardは、公開していない一時ファイルを消し、開いているディレクトリを閉じる。Publishの
// あとや2回目の呼び出しでは何もしない。
func (f *StagedFile) Discard() {
	if f.finished {
		return
	}
	f.finished = true
	f.temporary.remove()
	f.temporary.release()
}

// RemoveLeftoverStagedFilesは、StageFileがdirectoryにprefixで作り、PublishもDiscardも
// されずに残った一時ファイルを消す。StageFileの途中でプロセスが落ちると、一時ファイルは
// こうして残る。".sshc-"で始まる名前は一覧にも同期にも出ないので、消さなければ、見えない
// まま容量を使い続ける。
//
// 書いている途中の一時ファイルも同じ名前なので、呼び出し側は、同じdirectoryとprefixで
// StageFileを呼ぶ者が居ないとき（sshcエンジンの起動時など）にだけ呼ぶ。prefixは、
// IsTemporaryNameが認める".sshc-"のあとに用途の名前を続けたものに限る。".sshc-"だけでは、
// 変更の記録が復旧に使う一時ファイル（removeLeftoverTempsを参照）まで消してしまう。
func RemoveLeftoverStagedFiles(fileSystem FileSystem, directory, prefix string) error {
	if !IsTemporaryName(prefix) || len(prefix) <= len(temporaryPrefix) {
		return os.ErrInvalid
	}
	entries, err := fileSystem.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		err := fileSystem.Remove(filepath.Join(directory, entry.Name()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// copyAndFlushは、request.Sourceをfileへrequest.Maximumバイトまで書き、権限を与えて
// ディスクへフラッシュする。書いたバイト数を返す。
//
// 1バイト余分に読む。ちょうど上限で切ると、超えていることとちょうど収まっていることが
// 見分けられない。
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

// sourceReaderは、元の読み取りで起きたエラーを覚えておく。io.Copyは読み取りと書き込みの
// どちらで失敗したかを区別しないので、その区別をここで残す。
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

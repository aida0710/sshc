package selfupdate

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"sshc/internal/storage"
)

// LogFileName は、更新ジョブの間にインストーラーと再起動の helper が書いたものを、
// ジョブの状態（StateFileName）の隣に残すファイルである。状態のファイルには出力を
// 入れない。状態は小さく保って厳密に読み、出力は大きさの決まらない文として別に扱う。
const LogFileName = "web-update.log"

// LogCapacity は、ひとつのジョブについて記録する出力の上限である。install.sh、
// install.ps1、brew upgrade が書くのは数 KiB で、依存の formula をまとめて上げても
// 数十 KiB に収まる。上限を置くのは、インストーラーが延々と書いても、このマシンの
// ディスクを使い切らないためである。上限を超えた分は記録しない。
const LogCapacity = 256 << 10

// logHeaderPrefix に続けてジョブ ID と改行を書いた行が、ログの先頭に付く。ジョブ ID は
// 固定長なので、見出しの長さも固定であり、出力の位置は見出しの後ろから数える。
const logHeaderPrefix = "sshc-web-update-log "

const logHeaderLength = len(logHeaderPrefix) + jobIDLength + 1

// The workspace already excludes .sshc- temporary files from remote sync.
const temporaryLogPrefix = ".sshc-web-update-log-"

// LogPath は、statePath のジョブの出力を書くファイルである。
func LogPath(statePath string) string {
	return filepath.Join(filepath.Dir(statePath), LogFileName)
}

func logHeader(jobID string) []byte {
	return []byte(logHeaderPrefix + jobID + "\n")
}

// createLog は、jobID の出力を書くファイルを、見出しだけを持つ形で作り直す。前のジョブの
// 出力は捨てる。見出しごと置き換えるので、読む側が見出しの無いファイルを見ることはない。
func createLog(path, jobID string) error {
	return storage.WriteAtomicFile(storage.OSFileSystem{}, path, temporaryLogPrefix, storage.FilePermission, logHeader(jobID))
}

// LogWriter は、ひとつのジョブのログの末尾へ書き足す。LogCapacity を超えた分は捨て、
// それでも書けたと答える。ログのために、インストーラーや再起動を失敗させないためである。
// file が nil の LogWriter は、書いたものをすべて捨てる。
type LogWriter struct {
	mutex sync.Mutex
	file  *os.File
	// written は、見出しを除いて、ファイルにある出力の長さである。
	written int64
}

// OpenLog は、statePath の隣にある jobID のログを、書き足すために開く。ログが無い、
// 別のジョブのものである、読めない、のどれかなら、書いたものを捨てる LogWriter を返す。
// 新しいジョブがログを置き換えた後に、前のジョブの helper が書き足すこともない。
func OpenLog(statePath, jobID string) *LogWriter {
	file, err := os.OpenFile(LogPath(statePath), os.O_RDWR|os.O_APPEND, storage.FilePermission)
	if err != nil {
		return &LogWriter{}
	}
	header := make([]byte, logHeaderLength)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return &LogWriter{}
	}
	if _, err := io.ReadFull(file, header); err != nil || string(header) != string(logHeader(jobID)) {
		file.Close()
		return &LogWriter{}
	}
	return &LogWriter{file: file, written: info.Size() - int64(logHeaderLength)}
}

// Write は、標準出力と標準エラーの両方から呼ばれても、ひとつの書き込みを混ぜずに足す。
func (writer *LogWriter) Write(p []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	room := int64(LogCapacity) - writer.written
	if room <= 0 || writer.file == nil {
		return len(p), nil
	}
	kept := p
	if int64(len(kept)) > room {
		kept = kept[:room]
	}
	written, err := writer.file.Write(kept)
	writer.written += int64(written)
	if err != nil {
		// 書けなかったファイルには、もう書かない。続きの出力が途中から始まらないようにする。
		writer.closeLocked()
	}
	return len(p), nil
}

// Close は、ファイルを閉じる。二度呼んでもよい。
func (writer *LogWriter) Close() error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.closeLocked()
}

func (writer *LogWriter) closeLocked() error {
	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	return err
}

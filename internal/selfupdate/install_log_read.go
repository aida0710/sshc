package selfupdate

import (
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"sshc/internal/terminal"
)

var (
	// ErrNoLog は、いまのジョブのログが無いことを表す。ジョブがまだ無い、ログを作れなかった、
	// ログが前のジョブのままである、のどれかである。
	ErrNoLog = errors.New("no update log for the current job")
	// ErrLogCursorAhead は、読む位置が、ログの出力の終わりより先にあることを表す。
	// 前のジョブの位置を渡されたときに、これからの出力を黙って読み飛ばさないために断る。
	ErrLogCursorAhead = errors.New("the cursor is past the end of the update log")
)

// LogRead は、いまのジョブのログのうち、読んだ範囲である。位置は見出しを除いた出力の
// 先頭から数えるので、sshc エンジンが再起動しても変わらない。
type LogRead struct {
	JobID string
	// Start は読み始めた位置で、Next はその次に読む位置、End はいま書かれている出力の終わりである。
	Start, Next, End uint64
	// ReachedCapacity は、出力が LogCapacity に達し、それより後は記録していないことを表す。
	ReachedCapacity bool
	// Output は、[Start, Next) を、端末の制御文字を除いた文にしたものである。
	Output string
}

// Log は、いまのジョブのログを cursor から limit バイトまで読む。
func (service *Service) Log(cursor uint64, limit int) (LogRead, error) {
	job, err := service.store.Read()
	if err != nil {
		return LogRead{}, err
	}
	if job.ID == "" {
		return LogRead{}, ErrNoLog
	}
	content, err := readLogContent(LogPath(service.dependencies.StatePath), job.ID)
	if err != nil {
		return LogRead{}, err
	}
	end := uint64(len(content))
	if cursor > end {
		return LogRead{}, ErrLogCursorAhead
	}
	next := end
	if limit >= 0 && uint64(limit) < end-cursor {
		next = cursor + uint64(limit)
	}
	// 書き足される最中のログは、多バイト文字や CR LF の途中で終わっていることがある。
	// そこで切ると、続きを次に読んだときに文字が欠けたり改行が二つになったりするので、
	// 続きが来うるあいだは、切れ目の手前までを返す。
	if next < end || job.Active() {
		next = cursor + uint64(completeTextLength(content[cursor:next]))
	}
	return LogRead{
		JobID: job.ID, Start: cursor, Next: next, End: end,
		ReachedCapacity: end >= LogCapacity,
		// 先頭から decode するのは、cursor が端末の制御の途中に落ちても正しく読み飛ばすためである。
		// 出力は LogCapacity までなので、毎回先頭から読んでも小さい。
		Output: terminal.PlainTextFrom(content[:next], int(cursor)),
	}, nil
}

// readLogContent は、jobID のログから、見出しを除いた出力を読む。
func readLogContent(path, jobID string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoLog
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	header := make([]byte, logHeaderLength)
	if _, err := io.ReadFull(file, header); err != nil || string(header) != string(logHeader(jobID)) {
		return nil, ErrNoLog
	}
	// 上限より 1 バイトだけ多く読めば、上限を超えて書かれていないことも分かる。
	content, err := io.ReadAll(io.LimitReader(file, LogCapacity+1))
	if err != nil {
		return nil, err
	}
	if len(content) > LogCapacity {
		content = content[:LogCapacity]
	}
	return content, nil
}

// completeTextLength は、text の末尾から、書きかけの多バイト文字と、LF を待っている CR を
// 除いた長さを返す。
func completeTextLength(text []byte) int {
	length := len(text)
	start := length - 1
	for start >= 0 && length-start < utf8.UTFMax && !utf8.RuneStart(text[start]) {
		start--
	}
	if start >= 0 && !utf8.FullRune(text[start:length]) {
		length = start
	}
	if length > 0 && text[length-1] == '\r' {
		length--
	}
	return length
}

package sessionlog

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"

	"sshc/internal/privatefile"
	"sshc/internal/terminal"
	"sshc/third_party/vt"
)

// MaxFileBytes bounds one session's log. A program that prints without end must not fill the disk; the log says
// where it stopped.
const MaxFileBytes = 100 << 20

// maxPendingBytes bounds the output waiting to be written. Writing follows the terminal, so it falls behind only when
// a program prints faster than the disk takes it; what does not fit is left out, and the log says so.
const maxPendingBytes = 8 << 20

// redaction stands in the log for a secret the shell echoed back, as the snippets' redacted text does.
const redaction = "[secret]"

const (
	stoppedAtLimitNote = "ログが上限（100 MiB）に達したので、ここから先は書きません。"
	droppedOutputNote  = "出力が速すぎたため、ログに書けなかった出力があります。"
)

// Log is the log of one session: the lines its terminal showed, as text, in a file of its own. Lines are written once
// they leave the screen (scrolled off, or erased), so a program that redraws its rows leaves only what it drew last;
// a full-screen program leaves nothing. The rest of the screen is written when the log closes. Its methods do not
// wait for the disk: what they hand over is written in order by the log's own goroutine.
type Log struct {
	mutex        sync.Mutex
	pending      []entry
	pendingBytes int
	// dropped is set while output was left out, so the log says so once.
	dropped bool
	closed  bool
	wake    chan struct{}
	done    chan struct{}
}

type entry struct {
	output  []byte
	size    *terminal.Size
	note    string
	secrets []string
}

// FileName is the name of the log of a session to name, started at started, whose ID is id: the name, the time it
// started, and the start of its ID, so two sessions started together keep separate logs.
func FileName(name, id string, started time.Time) string {
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return safeFileName(name) + "-" + started.Format("20060102-150405") + "-" + safeFileName(short) + ".log"
}

// safeFileName keeps the characters every system takes in a file name, and stands _ for the others (/, :, spaces).
func safeFileName(name string) string {
	var built strings.Builder
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9',
			character == '.', character == '-', character == '_', character == '@':
			built.WriteRune(character)
		default:
			built.WriteRune('_')
		}
		if built.Len() >= 64 {
			break
		}
	}
	if built.Len() == 0 {
		return "session"
	}
	return built.String()
}

// Open creates the log file at path, and the folders on the way, readable only by their owner, and starts writing it
// for a terminal of size.
func Open(path string, size terminal.Size) (*Log, error) {
	file, err := privatefile.OpenAppend(path)
	if err != nil {
		return nil, err
	}
	log := &Log{wake: make(chan struct{}, 1), done: make(chan struct{})}
	go log.write(file, size)
	return log, nil
}

// Write hands over output the terminal showed.
func (l *Log) Write(output []byte) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.closed {
		return
	}
	if l.pendingBytes+len(output) > maxPendingBytes {
		l.dropped = true
		return
	}
	if l.dropped {
		l.pending = append(l.pending, entry{note: droppedOutputNote})
		l.dropped = false
	}
	l.pendingBytes += len(output)
	l.pending = append(l.pending, entry{output: append([]byte(nil), output...)})
	l.signal()
}

// Resize follows the terminal's new size, which decides where lines wrap.
func (l *Log) Resize(size terminal.Size) {
	l.add(entry{size: &size})
}

// Note writes an [sshc] line that says when something happened to the session (connected, disconnected).
func (l *Log) Note(text string) {
	l.add(entry{note: text})
}

// Hide keeps secrets out of the lines written from now on, for the values sshc types into the shell (snippet secrets)
// that the shell may echo back.
func (l *Log) Hide(secrets []string) {
	l.add(entry{secrets: append([]string(nil), secrets...)})
}

// Close writes what is still on the screen and closes the file, once what was handed over is written.
func (l *Log) Close() {
	l.mutex.Lock()
	if l.closed {
		l.mutex.Unlock()
		<-l.done
		return
	}
	l.closed = true
	l.signal()
	l.mutex.Unlock()
	<-l.done
}

func (l *Log) add(item entry) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if !l.closed {
		l.pending = append(l.pending, item)
		l.signal()
	}
}

func (l *Log) signal() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// take hands the writer what is waiting, and whether the log is closing with nothing more to come.
func (l *Log) take() ([]entry, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	taken := l.pending
	l.pending = nil
	l.pendingBytes = 0
	return taken, l.closed
}

// write is the log's goroutine: it follows the output on a screen of its own and writes the lines that leave it.
func (l *Log) write(file *os.File, size terminal.Size) {
	defer close(l.done)
	lines := lineWriter{file: file, buffered: bufio.NewWriter(file), now: time.Now}
	screen := vt.NewLeavingLinesEmulator(int(size.Cols), int(size.Rows))
	for {
		<-l.wake
		taken, closing := l.take()
		for _, item := range taken {
			switch {
			case item.output != nil:
				_, _ = screen.Write(item.output)
				lines.writeLines(screen.TakeLeftLines())
			case item.size != nil:
				screen.Resize(int(item.size.Cols), int(item.size.Rows))
				lines.writeLines(screen.TakeLeftLines())
			case item.secrets != nil:
				lines.secrets = append(lines.secrets, item.secrets...)
			default:
				lines.addNote(item.note, screen.MainScreenRowsWithText())
			}
		}
		lines.flush()
		if closing {
			lines.writeLines(screen.MainScreenLines())
			lines.writeWaitingNotes()
			lines.close()
			return
		}
	}
}

// lineWriter writes the log's lines to its file, until it is full.
type lineWriter struct {
	file     *os.File
	buffered *bufio.Writer
	now      func() time.Time
	secrets  []string
	written  int
	full     bool
	// notes wait, in order, for the lines that were on the screen when they were made: a line is written when it
	// leaves the screen, and a note must come after what was shown before it.
	notes []waitingNote
}

type waitingNote struct {
	line string
	// before is how many more lines leave the screen before the note is written.
	before int
}

func (w *lineWriter) writeLines(texts []string) {
	for _, text := range texts {
		w.writeLine(w.hideSecrets(text))
		for index := range w.notes {
			w.notes[index].before--
		}
		for len(w.notes) > 0 && w.notes[0].before <= 0 {
			w.writeLine(w.notes[0].line)
			w.notes = w.notes[1:]
		}
	}
}

// addNote writes an [sshc] line stamped now, once the onScreen rows of text shown before it have left the screen.
func (w *lineWriter) addNote(text string, onScreen int) {
	line := "[sshc] " + w.now().Format("2006-01-02 15:04:05") + " " + text
	if onScreen <= 0 && len(w.notes) == 0 {
		w.writeLine(line)
		return
	}
	w.notes = append(w.notes, waitingNote{line: line, before: onScreen})
}

// writeWaitingNotes writes the notes still waiting, as the log closes with nothing more to leave the screen.
func (w *lineWriter) writeWaitingNotes() {
	for _, note := range w.notes {
		w.writeLine(note.line)
	}
	w.notes = nil
}

func (w *lineWriter) writeLine(text string) {
	if w.full {
		return
	}
	if w.written+len(text)+1 > MaxFileBytes {
		w.full = true
		_, _ = w.buffered.WriteString("[sshc] " + stoppedAtLimitNote + "\n")
		return
	}
	n, _ := w.buffered.WriteString(text + "\n")
	w.written += n
}

func (w *lineWriter) hideSecrets(text string) string {
	for _, secret := range w.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, redaction)
		}
	}
	return text
}

func (w *lineWriter) flush() { _ = w.buffered.Flush() }

func (w *lineWriter) close() {
	w.flush()
	_ = w.file.Close()
}

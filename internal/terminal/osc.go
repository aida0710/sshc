package terminal

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Notification is one desktop-style notification a program running in the
// terminal asked for through OSC 9, OSC 99 or OSC 777, or the end of a long
// command the shell reported through OSC 133.
type Notification struct {
	Title      string
	Body       string
	OccurredAt time.Time
	// FinishedCommand is set, and Title and Body are empty, for a long command.
	FinishedCommand *FinishedCommand
}

const (
	// MaxOSCPayload bounds one observed sequence, so arbitrary terminal output
	// cannot grow memory. Past it a notification (OSC 9, OSC 777) keeps its
	// beginning, which is longer than what is shown anyway; any other sequence
	// is ignored, since a cut title or directory would be a wrong one.
	MaxOSCPayload = 4096
	// MaxNotificationTitleRunes and MaxNotificationBodyRunes keep the copy
	// shown in the session list and browser notifications short.
	MaxNotificationTitleRunes = 160
	MaxNotificationBodyRunes  = 512
	// maxKittyChunkBytes bounds what a multi-part kitty notification may
	// accumulate per field before it is delivered. Anything past the delivered
	// rune limits is discarded, so a stream of d=0 chunks cannot grow memory.
	maxKittyChunkBytes = 4 * MaxNotificationBodyRunes
	// maxKittyPendingIdentifiers bounds how many kitty notifications, told
	// apart by their i= identifier, may be buffered at once. A program usually
	// builds one notification at a time, so a few identifiers cover senders that
	// interleave. When one more identifier arrives, every pending notification
	// is dropped, which caps the buffered text at
	// maxKittyPendingIdentifiers * 2 * maxKittyChunkBytes (title and body).
	maxKittyPendingIdentifiers = 8
	// maxOSCNumberDigits bounds the number read before ';'. Every number this
	// observer handles (0, 1, 2, 7, 9, 99, 133, 777) fits; a longer one is skipped to the
	// end of its sequence without buffering a payload.
	maxOSCNumberDigits = 4
)

type oscState uint8

const (
	oscText oscState = iota
	oscEscape
	oscNumber
	oscPayload
	oscPayloadEscape
	oscDiscard
	oscDiscardEscape
)

// oscObserver watches the output stream for titles, notifications, program
// progress and the working directory a shell reports. It never removes bytes: the browser terminal still receives
// everything and applies these sequences itself. The engine only records what
// it saw so the session list, other tabs and headless clients can show it.
type oscObserver struct {
	state   oscState
	number  []byte
	payload []byte
	// truncated is set once the payload reached MaxOSCPayload and the rest of
	// the sequence was not kept.
	truncated       bool
	chunks          map[string]*kittyNotification
	reports         oscReports
	programProgress *ProgramProgress
}

// oscReports receives what the observer saw. A nil field ignores that kind.
type oscReports struct {
	Title     func(title string)
	Notify    func(title, body string)
	Progress  func(*ProgramProgress)
	Directory func(directory string)
	// CommandStarted and CommandFinished follow the shell integration's
	// OSC 133;C and OSC 133;D. exitCode is nil when the shell sent none.
	CommandStarted  func()
	CommandFinished func(exitCode *int)
}

type kittyNotification struct {
	title string
	body  string
}

func newOSCObserver(reports oscReports) *oscObserver {
	return &oscObserver{reports: reports}
}

// Observe scans one chunk. Sequences may span chunk boundaries.
func (o *oscObserver) Observe(chunk []byte) {
	for _, value := range chunk {
		switch o.state {
		case oscText:
			if value == '\x1b' {
				o.state = oscEscape
			}
		case oscEscape:
			switch value {
			case ']':
				o.state = oscNumber
				o.number = o.number[:0]
			case '\x1b':
			default:
				o.state = oscText
			}
		case oscNumber:
			switch {
			case value >= '0' && value <= '9' && len(o.number) < maxOSCNumberDigits:
				o.number = append(o.number, value)
			case value == ';' && len(o.number) > 0:
				o.state = oscPayload
				o.payload = o.payload[:0]
			case value == '\a':
				o.reset()
			case value == '\x1b':
				o.state = oscDiscardEscape
			default:
				// Not a sequence this observer understands. Skip to its end
				// so a stray ';' inside cannot start a bogus payload.
				o.state = oscDiscard
			}
		case oscPayload:
			switch value {
			case '\a':
				o.complete()
			case '\x1b':
				o.state = oscPayloadEscape
			default:
				if len(o.payload) >= MaxOSCPayload {
					o.truncated = true
					continue
				}
				o.payload = append(o.payload, value)
			}
		case oscPayloadEscape:
			if value == '\\' {
				o.complete()
			} else {
				// A terminal aborts the string on any other escape, so the
				// byte after ESC begins a new sequence.
				o.abandon(value)
			}
		case oscDiscard:
			switch value {
			case '\a':
				o.reset()
			case '\x1b':
				o.state = oscDiscardEscape
			}
		case oscDiscardEscape:
			if value == '\\' {
				o.reset()
			} else {
				o.abandon(value)
			}
		}
	}
}

// abandon drops the sequence in progress and re-reads value as the byte that
// followed an ESC.
func (o *oscObserver) abandon(value byte) {
	o.reset()
	switch value {
	case ']':
		o.state = oscNumber
	case '\x1b':
		o.state = oscEscape
	}
}

func (o *oscObserver) reset() {
	o.state = oscText
	o.number = o.number[:0]
	o.payload = o.payload[:0]
	o.truncated = false
}

func (o *oscObserver) complete() {
	number, payload, truncated := string(o.number), string(o.payload), o.truncated
	o.reset()
	if truncated && number != "9" && number != "777" {
		return
	}
	switch number {
	case "0", "1", "2":
		if o.reports.Title != nil {
			o.reports.Title(cleanDisplayText(payload, MaxTitle))
		}
	case "7":
		if directory, ok := parseOSC7Directory(payload); ok && o.reports.Directory != nil {
			o.reports.Directory(directory)
		}
	case "9":
		if strings.HasPrefix(payload, "4;") {
			o.reportProgramProgress(nextProgramProgress(o.programProgress, payload))
		} else {
			o.notifyITerm(payload)
		}
	case "99":
		o.notifyKitty(payload)
	case "133":
		kind, detail, _ := strings.Cut(payload, ";")
		switch kind {
		case "C":
			if o.reports.CommandStarted != nil {
				o.reports.CommandStarted()
			}
		case "D":
			// A program that quit without clearing its OSC 9;4 progress leaves
			// no bar behind once the shell reports the command finished.
			o.reportProgramProgress(nil)
			if o.reports.CommandFinished != nil {
				o.reports.CommandFinished(commandExitCode(detail))
			}
		}
	case "777":
		o.notifyRxvt(payload)
	}
}

// commandExitCode reads the exit code of `OSC 133 ; D ; code`. The key=value
// fields some shells add after it are ignored.
func commandExitCode(detail string) *int {
	field, _, _ := strings.Cut(detail, ";")
	code, err := strconv.Atoi(field)
	if err != nil {
		return nil
	}
	return &code
}

func (o *oscObserver) reportProgramProgress(progress *ProgramProgress) {
	o.programProgress = progress
	if o.reports.Progress != nil {
		o.reports.Progress(progress)
	}
}

// notifyITerm handles the iTerm2 form `OSC 9 ; message`. ConEmu uses the same
// number for progress bars and other controls whose payload begins with a
// digit and ';' — those are not notifications.
func (o *oscObserver) notifyITerm(payload string) {
	if index := strings.IndexByte(payload, ';'); index >= 0 && isDigits(payload[:index]) {
		return
	}
	o.emit("", payload)
}

// notifyRxvt handles `OSC 777 ; notify ; title ; body`. Other 777 commands
// are ignored.
func (o *oscObserver) notifyRxvt(payload string) {
	parts := strings.SplitN(payload, ";", 3)
	if len(parts) < 2 || parts[0] != "notify" {
		return
	}
	body := ""
	if len(parts) == 3 {
		body = parts[2]
	}
	o.emit(parts[1], body)
}

// notifyKitty handles `OSC 99 ; metadata ; payload`. A notification may
// arrive in several chunks that share an identifier; chunks with d=0 are
// buffered until the closing chunk. Payloads without metadata are treated
// as the message body so the simplified form `OSC 99 ; message` also works.
func (o *oscObserver) notifyKitty(payload string) {
	metadata, text, hasMetadata := strings.Cut(payload, ";")
	if !hasMetadata || !looksLikeKittyMetadata(metadata) {
		o.emit("", payload)
		return
	}
	identifier, part, done, encoded := "0", "body", true, false
	for _, field := range strings.Split(metadata, ":") {
		key, value, _ := strings.Cut(field, "=")
		switch key {
		case "i":
			identifier = value
		case "p":
			part = value
		case "d":
			done = value != "0"
		case "e":
			encoded = value == "1"
		}
	}
	if encoded {
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return
		}
		text = string(decoded)
	}
	if o.chunks == nil {
		o.chunks = make(map[string]*kittyNotification)
	}
	pending := o.chunks[identifier]
	if pending == nil {
		if len(o.chunks) >= maxKittyPendingIdentifiers {
			o.chunks = make(map[string]*kittyNotification)
		}
		pending = &kittyNotification{}
		o.chunks[identifier] = pending
	}
	switch part {
	case "title":
		pending.title = appendBounded(pending.title, text, maxKittyChunkBytes)
	case "body":
		pending.body = appendBounded(pending.body, text, maxKittyChunkBytes)
	}
	if !done {
		return
	}
	delete(o.chunks, identifier)
	o.emit(pending.title, pending.body)
}

// appendBounded は existing に text を足し、limit バイトを超える分を捨てる。
func appendBounded(existing, text string, limit int) string {
	room := limit - len(existing)
	if room <= 0 {
		return existing
	}
	if len(text) > room {
		text = text[:room]
	}
	return existing + text
}

func looksLikeKittyMetadata(metadata string) bool {
	if metadata == "" {
		return true
	}
	for _, field := range strings.Split(metadata, ":") {
		key, _, found := strings.Cut(field, "=")
		if !found || key == "" || strings.ContainsAny(key, " \t") {
			return false
		}
	}
	return true
}

func (o *oscObserver) emit(title, body string) {
	title = cleanDisplayText(title, MaxNotificationTitleRunes)
	body = cleanDisplayText(body, MaxNotificationBodyRunes)
	if title == "" && body == "" {
		return
	}
	if o.reports.Notify != nil {
		o.reports.Notify(title, body)
	}
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// DisplayText keeps text that came from a remote program safe to show:
// control and format characters are dropped and the length is bounded.
// Authentication prompts and OSC titles both pass through here so a server
// cannot smuggle escape sequences into the pane.
func DisplayText(value string, maximum int) string {
	return cleanDisplayText(value, maximum)
}

// cleanDisplayText keeps text that came from a remote program safe to show:
// control and format characters are dropped and the length is bounded.
func cleanDisplayText(value string, maximum int) string {
	value = strings.TrimSpace(norm.NFC.String(value))
	if value == "" {
		return ""
	}
	var cleaned strings.Builder
	count := 0
	for _, character := range value {
		if character == utf8.RuneError || unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			continue
		}
		cleaned.WriteRune(character)
		count++
		if count >= maximum {
			break
		}
	}
	return strings.TrimSpace(cleaned.String())
}

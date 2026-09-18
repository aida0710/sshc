package terminal

import (
	"encoding/base64"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Notification is one desktop-style notification a program running in the
// terminal asked for through OSC 9, OSC 99 or OSC 777.
type Notification struct {
	Title      string
	Body       string
	OccurredAt time.Time
}

const (
	// MaxOSCPayload bounds one observed sequence. Longer payloads are ignored
	// so arbitrary terminal output cannot grow memory.
	MaxOSCPayload = 4096
	// MaxNotificationTitleRunes and MaxNotificationBodyRunes keep the copy
	// shown in the session list and browser notifications short.
	MaxNotificationTitleRunes = 160
	MaxNotificationBodyRunes  = 512
	// maxKittyChunkBytes bounds what a multi-part kitty notification may
	// accumulate per field before it is delivered. Anything past the delivered
	// rune limits is discarded, so a stream of d=0 chunks cannot grow memory.
	maxKittyChunkBytes = 4 * MaxNotificationBodyRunes
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

// oscObserver watches the output stream for the standard title and
// notification sequences. It never removes bytes: the browser terminal still
// receives everything and applies titles itself. The engine only records what
// it saw so the session list, other tabs and headless clients can show it.
type oscObserver struct {
	state    oscState
	number   []byte
	payload  []byte
	chunks   map[string]*kittyNotification
	onTitle  func(title string)
	onNotify func(title, body string)
}

type kittyNotification struct {
	title string
	body  string
}

func newOSCObserver(onTitle func(string), onNotify func(title, body string)) *oscObserver {
	return &oscObserver{onTitle: onTitle, onNotify: onNotify}
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
			case value >= '0' && value <= '9' && len(o.number) < 4:
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
					o.state = oscDiscard
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
}

func (o *oscObserver) complete() {
	number, payload := string(o.number), string(o.payload)
	o.reset()
	switch number {
	case "0", "1", "2":
		if o.onTitle != nil {
			o.onTitle(cleanDisplayText(payload, MaxTitle))
		}
	case "9":
		o.notifyITerm(payload)
	case "99":
		o.notifyKitty(payload)
	case "777":
		o.notifyRxvt(payload)
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
		if len(o.chunks) >= 8 {
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
	if o.onNotify != nil {
		o.onNotify(title, body)
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

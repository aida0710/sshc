package terminal

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Program status (OSC 7501, https://control-codes.page/proposals/osc-7501/,
// revision 0.4) lets a program say what it is doing: at rest, working, blocked
// on the user, done with a result nobody has seen yet, or failed. A program
// may keep several records, told apart by a slash-separated id. The engine
// keeps them per session, so every browser and the session list show the same
// state; xterm.js in the browser ignores the sequence.

// ProgramStatusState is what a program said it is doing.
type ProgramStatusState string

const (
	ProgramStatusIdle    ProgramStatusState = "idle"
	ProgramStatusWorking ProgramStatusState = "working"
	ProgramStatusBlocked ProgramStatusState = "blocked"
	ProgramStatusDone    ProgramStatusState = "done"
	ProgramStatusError   ProgramStatusState = "error"
)

// ProgramStatusKind is what a blocked program waits for.
type ProgramStatusKind string

const (
	ProgramStatusPermission ProgramStatusKind = "permission"
	ProgramStatusQuestion   ProgramStatusKind = "question"
	ProgramStatusAuth       ProgramStatusKind = "auth"
)

// ProgramStatus is one record as the session list shows it.
type ProgramStatus struct {
	State ProgramStatusState
	// Kind is only ever set on a blocked record, and not always then.
	Kind ProgramStatusKind
	// Progress is 0 to 100 on a working or blocked record; nil is indeterminate.
	Progress *int
	// App is the program's stable name (cargo, make), from the record or from
	// its nearest ancestor that has one.
	App     string
	Title   string
	Message string
	// Version is the update number of the report that set the record. It only
	// grows within a session, so a browser can tell a record it has not shown.
	Version uint64
}

// The limits are the spec's. A report past one is dropped whole.
const (
	// maxProgramStatusRecords is the fewest records the spec lets a terminal
	// keep. Past it the least recently updated one makes way.
	maxProgramStatusRecords      = 64
	maxProgramStatusKeyBytes     = 16
	maxProgramStatusAppBytes     = 32
	maxProgramStatusIDBytes      = 128
	maxProgramStatusIDSegments   = 8
	maxProgramStatusSegmentBytes = 32
	maxProgramStatusTitleBytes   = 192
	maxProgramStatusMessageBytes = 2048
	maxProgramStatusPercent      = 100
)

// programStatusAnswer answers OSC 7501 ; ?. Listing no states and no kinds
// means all five states and all three kinds. It holds nothing from a report.
const programStatusAnswer = "\x1b]7501;?\x1b\\"

// programStatusReport is one OSC 7501 sequence the observer read.
type programStatusReport struct {
	// query asks whether the terminal knows the protocol (OSC 7501 ; ?).
	query bool
	// clear removes the record id and every record beneath it; with id "",
	// every record.
	clear bool
	id    string
	// status.App is the record's own name, before any is taken from an ancestor.
	status ProgramStatus
}

// parseProgramStatus reads the payload after "7501;". ok is false for a report
// to drop whole: no known state, a limit broken, an id with a byte the spec
// does not allow, or text that is not base64 of plain UTF-8.
func parseProgramStatus(payload string) (report programStatusReport, ok bool) {
	pairs := strings.Split(payload, ":")
	if strings.TrimSpace(pairs[0]) == "?" {
		return programStatusReport{query: true}, true
	}
	values := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(key) > maxProgramStatusKeyBytes {
			return programStatusReport{}, false
		}
		if !found || !isProgramStatusKey(key) {
			continue
		}
		if key == "id" {
			if !isProgramStatusID(value) {
				return programStatusReport{}, false
			}
		} else if !onlyBytesIn(value, isProgramStatusValueByte) {
			// A malformed pair is skipped; the rest of the report applies.
			continue
		}
		values[key] = value
	}

	report.id = values["id"]
	state := ProgramStatusState(values["state"])
	switch state {
	case ProgramStatusIdle, ProgramStatusWorking, ProgramStatusBlocked, ProgramStatusDone, ProgramStatusError:
	case "clear":
		report.clear = true
		return report, true
	default:
		return programStatusReport{}, false
	}

	app := values["app"]
	if len(app) > maxProgramStatusAppBytes {
		return programStatusReport{}, false
	}
	if !onlyBytesIn(app, isProgramStatusAppByte) {
		app = ""
	}
	title, titleOK := decodeProgramStatusText(values["title"], maxProgramStatusTitleBytes)
	message, messageOK := decodeProgramStatusText(values["msg"], maxProgramStatusMessageBytes)
	if !titleOK || !messageOK {
		return programStatusReport{}, false
	}
	report.status = ProgramStatus{State: state, App: app, Title: title, Message: message}
	if state == ProgramStatusBlocked {
		switch kind := ProgramStatusKind(values["kind"]); kind {
		case ProgramStatusPermission, ProgramStatusQuestion, ProgramStatusAuth:
			report.status.Kind = kind
		}
	}
	if state == ProgramStatusWorking || state == ProgramStatusBlocked {
		report.status.Progress = programStatusPercent(values["progress"])
	}
	return report, true
}

func isProgramStatusKey(key string) bool {
	return key != "" && onlyBytesIn(key, func(value byte) bool { return value >= 'a' && value <= 'z' })
}

// isProgramStatusValueByte is the whole alphabet of a value, so no value
// needs escaping and base64 fits as it is.
func isProgramStatusValueByte(value byte) bool {
	return isASCIIAlphanumeric(value) || strings.IndexByte("_.,+/=-", value) >= 0
}

func isProgramStatusAppByte(value byte) bool {
	return isASCIIAlphanumeric(value) || strings.IndexByte("_.+-", value) >= 0
}

func isASCIIAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func onlyBytesIn(text string, allowed func(byte) bool) bool {
	for index := 0; index < len(text); index++ {
		if !allowed(text[index]) {
			return false
		}
	}
	return true
}

// isProgramStatusID reports whether id is a path the spec allows. "" is the
// root record. A segment may hold any byte of a value but the separator, and
// an empty segment is taken as a byte out of place.
func isProgramStatusID(id string) bool {
	if id == "" {
		return true
	}
	if len(id) > maxProgramStatusIDBytes {
		return false
	}
	segments := strings.Split(id, "/")
	if len(segments) > maxProgramStatusIDSegments {
		return false
	}
	for _, segment := range segments {
		if segment == "" || len(segment) > maxProgramStatusSegmentBytes || !onlyBytesIn(segment, isProgramStatusValueByte) {
			return false
		}
	}
	return true
}

// decodeProgramStatusText reads a title or msg: base64 of UTF-8, padding
// optional, with no control character at all (not even a newline or a tab).
// The text is shown outside the terminal, so format characters such as
// direction overrides are dropped too.
func decodeProgramStatusText(encoded string, maximum int) (string, bool) {
	if encoded == "" {
		return "", true
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(maximum) {
		return "", false
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
	if err != nil || len(decoded) > maximum || !utf8.Valid(decoded) {
		return "", false
	}
	text := string(decoded)
	if strings.ContainsFunc(text, unicode.IsControl) {
		return "", false
	}
	return cleanDisplayText(text, maximum), true
}

// programStatusPercent reads progress. Anything but 0 to 100 is indeterminate.
func programStatusPercent(value string) *int {
	if len(value) > len("100") || !isDigits(value) {
		return nil
	}
	percent, _ := strconv.Atoi(value)
	if percent > maxProgramStatusPercent {
		return nil
	}
	return &percent
}

package terminal

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// プログラムの状態（OSC 7501、https://control-codes.page/proposals/osc-7501/、revision 0.4）は、プログラムが
// 今していることを伝える仕組みである。休んでいる、働いている、利用者を待っている、まだ誰も見ていない結果を
// 残して終わった、失敗した、のどれかを言える。プログラムは、スラッシュで区切った id で見分ける記録を、
// いくつも持てる。エンジンは記録をセッションごとに持つので、どのブラウザもセッションの一覧も同じ状態を出す。
// ブラウザの xterm.js はこのシーケンスを無視する。

// ProgramStatusState は、プログラムが今していると言ったことである。
type ProgramStatusState string

const (
	ProgramStatusIdle    ProgramStatusState = "idle"
	ProgramStatusWorking ProgramStatusState = "working"
	ProgramStatusBlocked ProgramStatusState = "blocked"
	ProgramStatusDone    ProgramStatusState = "done"
	ProgramStatusError   ProgramStatusState = "error"
)

// ProgramStatusKind は、利用者を待っているプログラムが、何を待っているかである。
type ProgramStatusKind string

const (
	ProgramStatusPermission ProgramStatusKind = "permission"
	ProgramStatusQuestion   ProgramStatusKind = "question"
	ProgramStatusAuth       ProgramStatusKind = "auth"
)

// ProgramStatus は、セッションの一覧に出す形の記録1つである。
type ProgramStatus struct {
	State ProgramStatusState
	// Kind が入るのは blocked の記録だけで、そのときも入るとは限らない。
	Kind ProgramStatusKind
	// Progress は、working か blocked の記録で 0 から 100 である。nil は、どこまで進んだか分からないことを表す。
	Progress *int
	// App は、プログラムの変わらない名前（cargo、make）である。記録そのものか、名前を持ついちばん近い親の記録から
	// 取る。
	App     string
	Title   string
	Message string
	// Version は、記録を入れた報告の更新番号である。セッションの中では増えるだけなので、ブラウザは、まだ
	// 出していない記録を見分けられる。
	Version uint64
}

// 上限は仕様のものである。1つでも超えた報告は、丸ごと捨てる。
const (
	// maxProgramStatusRecords は、仕様がターミナルに持つよう求める記録の最少の数である。超えたら、更新が
	// いちばん古い記録を捨てる。
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

// programStatusAnswer は、OSC 7501 ; ? への答えである。状態も種類も挙げないのは、5つの状態と3つの種類を
// すべて扱うという意味である。報告から来たものは何も含まない。
const programStatusAnswer = "\x1b]7501;?\x1b\\"

// programStatusReport は、見張りが読んだ OSC 7501 のシーケンス1つである。
type programStatusReport struct {
	// query は、ターミナルがこの仕組みを知っているかを尋ねる（OSC 7501 ; ?）。
	query bool
	// clear は、id の記録と、その下のすべての記録を消す。id が "" なら、すべての記録を消す。
	clear bool
	id    string
	// status.App は、親から名前を取る前の、その記録自身の名前である。
	status ProgramStatus
}

// parseProgramStatus は、"7501;" のあとの本文を読む。丸ごと捨てる報告では ok が false になる。知らない状態、
// 上限の超過、仕様が許さないバイトを含む id、平文の UTF-8 の base64 でない文字が、それにあたる。
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
			// 形の崩れた組は飛ばす。報告の残りは当てる。
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

// isProgramStatusValueByte は、値に使える文字のすべてである。そのため、どの値もエスケープが要らず、base64 も
// そのまま入る。
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

// isProgramStatusID は、id が仕様の許すパスかを返す。"" は根の記録である。区切りを除いて、値に使えるバイトは
// どれも区切りの間に置ける。空の区切りの間は、場違いなバイトとして扱う。
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

// decodeProgramStatusText は、title か msg を読む。UTF-8 の base64 で、パディングは無くてもよく、制御文字は
// 1つも含まない（改行もタブも含まない）。この文字はターミナルの外に出すので、向きを上書きするような書式の
// 文字も落とす。
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

// programStatusPercent は、進み具合を読む。0 から 100 以外は、どこまで進んだか分からないものとして扱う。
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

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

// Notification は、ターミナルで走るプログラムが OSC 9、OSC 99、OSC 777 で求めたデスクトップ風の通知1つか、
// シェルが OSC 133 で報告した長いコマンドの終わりである。
type Notification struct {
	Title      string
	Body       string
	OccurredAt time.Time
	// FinishedCommand は、長いコマンドの終わりのときに入る。そのとき Title と Body は空である。
	FinishedCommand *FinishedCommand
}

const (
	// MaxOSCPayload は、見張るシーケンス1つの長さの上限である。ターミナルの任意の出力でメモリが増え続けない
	// ようにする。これを超えた通知（OSC 9、OSC 777）は先頭だけを残す。それでも表示する長さより長い。ほかの
	// シーケンスは無視する。途中で切れた名前やフォルダは誤ったものになるためである。
	MaxOSCPayload = 4096
	// MaxNotificationTitleRunes と MaxNotificationBodyRunes は、セッションの一覧とブラウザの通知に出す文を
	// 短く保つ。
	MaxNotificationTitleRunes = 160
	MaxNotificationBodyRunes  = 512
	// maxKittyChunkBytes は、分割された kitty の通知が、届け終えるまでに欄ごとに溜めてよい量の上限である。
	// 届ける文字数の上限を超えた分は捨てるので、d=0 のかたまりが続いてもメモリは増えない。
	maxKittyChunkBytes = 4 * MaxNotificationBodyRunes
	// maxKittyPendingIdentifiers は、i= の識別子で見分ける kitty の通知を、同時にいくつまで溜めてよいかの上限で
	// ある。プログラムはふつう通知を1つずつ組み立てるので、識別子が少しあれば、交互に送る送り手にも足りる。
	// もう1つ識別子が届いたら、溜めている通知をすべて捨てる。これで、溜める文字は
	// maxKittyPendingIdentifiers * 2 * maxKittyChunkBytes（名前と本文）を超えない。
	maxKittyPendingIdentifiers = 8
	// maxOSCNumberDigits は、';' の前に読む番号の桁数の上限である。この見張りが扱う番号
	// （0、1、2、7、9、99、133、777、7501）はすべて収まる。それより長い番号は、本文を溜めずに、シーケンスの
	// 終わりまで読み飛ばす。
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

// oscObserver は、出力の流れから、名前、通知、プログラムの進み具合と状態、シェルが報告する作業フォルダを
// 見張る。バイトは1つも取り除かない。ブラウザのターミナルはすべてを受け取り、これらのシーケンスを自分で
// 当てる。エンジンは見たものを記録するだけで、セッションの一覧、ほかのタブ、画面の無いクライアントが
// それを出せるようにする。
type oscObserver struct {
	state   oscState
	number  []byte
	payload []byte
	// truncated は、本文が MaxOSCPayload に達し、シーケンスの残りを持たなかったあと真になる。
	truncated       bool
	chunks          map[string]*kittyNotification
	reports         oscReports
	programProgress *ProgramProgress
}

// oscReports は、見張りが見たものを受け取る。nil の欄の種類は無視する。
type oscReports struct {
	Title     func(title string)
	Notify    func(title, body string)
	Progress  func(*ProgramProgress)
	Directory func(directory string)
	// PromptStarted、CommandStarted、CommandFinished は、シェル統合の OSC 133;A、OSC 133;C、OSC 133;D に
	// 従う。シェルが終了コードを送らなかったとき、exitCode は nil である。
	PromptStarted   func()
	CommandStarted  func()
	CommandFinished func(exitCode *int)
	// ProgramStatus は、捨てなかった OSC 7501 の報告と問い合わせを1つずつ受け取る（program_status.go）。
	ProgramStatus func(programStatusReport)
	// Reset は、ターミナルの全体のリセット（RIS、ESC c）に従う。
	Reset func()
}

type kittyNotification struct {
	title string
	body  string
}

func newOSCObserver(reports oscReports) *oscObserver {
	return &oscObserver{reports: reports}
}

// Observe は、かたまり1つを調べる。シーケンスは、かたまりの境をまたぐことがある。
func (o *oscObserver) Observe(chunk []byte) {
	for _, value := range chunk {
		switch o.state {
		case oscText:
			if value == '\x1b' {
				o.state = oscEscape
			}
		case oscEscape:
			o.afterEscape(value)
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
				// この見張りが知らないシーケンスである。中に紛れた ';' が偽の本文を始めないよう、終わりまで読み飛ばす。
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
				// ターミナルは、ほかのエスケープが来ると文字列を打ち切る。ESC の次のバイトは、新しいシーケンスの始まりである。
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

// afterEscape は、ESC に続いたバイトを読む。
func (o *oscObserver) afterEscape(value byte) {
	switch value {
	case ']':
		o.state = oscNumber
		o.number = o.number[:0]
	case 'c':
		o.state = oscText
		if o.reports.Reset != nil {
			o.reports.Reset()
		}
	case '\x1b':
		o.state = oscEscape
	default:
		o.state = oscText
	}
}

// abandon は、読んでいる途中のシーケンスを捨て、value を ESC に続いたバイトとして読み直す。
func (o *oscObserver) abandon(value byte) {
	o.reset()
	o.afterEscape(value)
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
			o.reports.Title(cleanDisplayText(withoutExecutablePath(payload), MaxTitle))
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
		case "A":
			if o.reports.PromptStarted != nil {
				o.reports.PromptStarted()
			}
		case "C":
			if o.reports.CommandStarted != nil {
				o.reports.CommandStarted()
			}
		case "D":
			// OSC 9;4 の進み具合を消さずに終わったプログラムがあっても、シェルがコマンドの終わりを報告したら、
			// バーを残さない。
			o.reportProgramProgress(nil)
			if o.reports.CommandFinished != nil {
				o.reports.CommandFinished(commandExitCode(detail))
			}
		}
	case "777":
		o.notifyRxvt(payload)
	case "7501":
		if report, ok := parseProgramStatus(payload); ok && o.reports.ProgramStatus != nil {
			o.reports.ProgramStatus(report)
		}
	}
}

// commandExitCode は、`OSC 133 ; D ; code` の終了コードを読む。その後ろにシェルが足す key=value の欄は
// 無視する。
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

// notifyITerm は、iTerm2 の形 `OSC 9 ; message` を扱う。ConEmu は同じ番号を、プログレスバーなどの制御に
// 使う。その本文は数字と ';' で始まり、通知ではない。
func (o *oscObserver) notifyITerm(payload string) {
	if index := strings.IndexByte(payload, ';'); index >= 0 && isDigits(payload[:index]) {
		return
	}
	o.emit("", payload)
}

// notifyRxvt は、`OSC 777 ; notify ; title ; body` を扱う。ほかの 777 のコマンドは無視する。
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

// notifyKitty は、`OSC 99 ; metadata ; payload` を扱う。通知は、同じ識別子を持ついくつかのかたまりで届く
// ことがある。d=0 のかたまりは、閉じるかたまりが来るまで溜める。metadata の無い本文は通知の本文として扱う
// ので、簡単な形 `OSC 99 ; message` も通る。
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

// DisplayText は、リモートのプログラムから来た文字を、出しても安全な形にする。制御文字と書式の文字を
// 落とし、長さを抑える。認証の問いも OSC の名前もここを通るので、サーバーはエスケープシーケンスをペインへ
// 紛れ込ませられない。
func DisplayText(value string, maximum int) string {
	return cleanDisplayText(value, maximum)
}

// cleanDisplayText は、リモートのプログラムから来た文字を、出しても安全な形にする。制御文字と書式の文字を
// 落とし、長さを抑える。
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

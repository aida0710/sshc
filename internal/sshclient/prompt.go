package sshclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

// ErrPromptAborted は、ユーザーが問いに応答せずに打ち切ったことを報告する。
var ErrPromptAborted = errors.New("the prompt was cancelled")

// ErrPromptUnavailable は、利用者へ質問できない接続で追加の入力が必要になった
// ことを報告する。保存済み資格情報はこのエラーより先に試される。
var ErrPromptUnavailable = errors.New("authentication requires user input, but this operation is non-interactive")

// Prompter は、接続の途中でユーザーに尋ねる手段である。
//
// 尋ねることは 4 つある。未知のホスト鍵を受け入れるか、鍵のパスフレーズ、
// パスワード、keyboard-interactive の質問。仕組みを 4 つ作らない。
// どれも「端末へ書いて、端末から読む」という同じことである。
type Prompter interface {
	// Line は打った文字が見える問い。
	Line(prompt string) (string, error)
	// Secret は値の代わりに1文字ずつ伏せ字を表示する問い。
	Secret(prompt string) (string, error)
	// Confirm は yes か no を求める。
	Confirm(prompt string) (bool, error)
}

// nonInteractivePrompter は、保存済み資格情報を対話接続と同じ認証経路へ
// 通しつつ、追加質問だけを拒否する。nil を渡すと Auth.Methods が password と
// keyboard-interactive 自体を除外し、vault に保存済みの結果まで使えなくなる。
type nonInteractivePrompter struct{}

func (nonInteractivePrompter) Line(string) (string, error) {
	return "", ErrPromptUnavailable
}

func (nonInteractivePrompter) Secret(string) (string, error) {
	return "", ErrPromptUnavailable
}

func (nonInteractivePrompter) Confirm(string) (bool, error) {
	return false, ErrPromptUnavailable
}

var noPrompt Prompter = nonInteractivePrompter{}

// StreamPrompter は、端末のストリームへ問いを出す。
//
// 端末は raw モードである（xterm.js はローカルエコーを持たない）。だから
// 見える問いのエコーはこちらが書く。秘密の問いでは値の代わりに伏せ字を
// 書く。結果を端末へ書き戻さないのは、それが画面にもスクロールバックにも
// 残るからである。
type StreamPrompter struct {
	Out io.Writer
	In  io.Reader
	// begin と end は、非同期SSH sessionが通常入力と認証回答を区別するための
	// 内部hookである。公開Prompterの利用者は設定しなくてよい。
	begin func()
	end   func()
}

type promptEcho uint8

const (
	promptEchoVisible promptEcho = iota
	promptEchoMasked
)

func (p StreamPrompter) Line(prompt string) (string, error) {
	return p.read(prompt, promptEchoVisible)
}

func (p StreamPrompter) Secret(prompt string) (string, error) {
	return p.read(prompt, promptEchoMasked)
}

// Confirm は yes か no だけを受ける。
//
// OpenSSH と同じで、y も Enter も結果にならない。ホスト鍵を受け入れるかどうかは
// 打ち間違いで通ってよい問いではない。
func (p StreamPrompter) Confirm(prompt string) (bool, error) {
	for attempt := 0; attempt < maxConfirmAttempts; attempt++ {
		answer, err := p.read(prompt, promptEchoVisible)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "yes":
			return true, nil
		case "no":
			return false, nil
		}
		if _, err := io.WriteString(p.Out, "Please type 'yes' or 'no': "); err != nil {
			return false, err
		}
		prompt = ""
	}
	return false, ErrPromptAborted
}

// maxConfirmAttempts は、同じ問いを繰り返す回数の上限である。上限が無いと、
// 応答しないクライアントがこの接続の goroutine を永久に保持する。
const maxConfirmAttempts = 3

// maxAnswer は、ひとつの結果の長さの上限である。
const maxAnswer = 1024

func (p StreamPrompter) read(prompt string, echo promptEcho) (string, error) {
	if p.begin != nil {
		p.begin()
	}
	if p.end != nil {
		defer p.end()
	}
	if prompt != "" {
		if _, err := io.WriteString(p.Out, prompt); err != nil {
			return "", err
		}
	}

	var answer []byte
	defer func() { clear(answer) }()
	displayedRunes := 0
	buffer := make([]byte, 1)
	for {
		read, err := p.In.Read(buffer)
		if read == 0 {
			if err == nil {
				continue
			}
			if errors.Is(err, io.EOF) {
				return "", ErrPromptAborted
			}
			return "", err
		}
		switch character := buffer[0]; character {
		case '\r', '\n':
			// ブラウザの端末は Enter を CR で送る。両方を行末として扱う。
			if _, err := io.WriteString(p.Out, "\r\n"); err != nil {
				return "", err
			}
			return string(answer), nil
		case 0x03, 0x04:
			// Ctrl-C と Ctrl-D。応答せずに打ち切ったという事実である。
			if _, err := io.WriteString(p.Out, "\r\n"); err != nil {
				return "", err
			}
			return "", ErrPromptAborted
		case 0x7f, 0x08:
			if len(answer) == 0 {
				continue
			}
			answer = removeLastInputCharacter(answer)
			if echo == promptEchoVisible {
				if _, err := io.WriteString(p.Out, "\b \b"); err != nil {
					return "", err
				}
			} else if err := writeMaskedPromptFeedback(p.Out, answer, &displayedRunes); err != nil {
				return "", err
			}
		case 0x15: // Ctrl-U
			if len(answer) == 0 {
				continue
			}
			removed := utf8.RuneCount(answer)
			clear(answer)
			answer = answer[:0]
			if echo == promptEchoVisible {
				if _, err := io.WriteString(p.Out, strings.Repeat("\b \b", removed)); err != nil {
					return "", err
				}
			} else if err := writeMaskedPromptFeedback(p.Out, answer, &displayedRunes); err != nil {
				return "", err
			}
		default:
			if character < 0x20 || len(answer) >= maxAnswer {
				continue
			}
			answer = append(answer, character)
			if echo == promptEchoVisible {
				if _, err := p.Out.Write(buffer); err != nil {
					return "", err
				}
			} else if err := writeMaskedPromptFeedback(p.Out, answer, &displayedRunes); err != nil {
				return "", err
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
	}
}

func removeLastInputCharacter(answer []byte) []byte {
	if !utf8.Valid(answer) {
		answer[len(answer)-1] = 0
		return answer[:len(answer)-1]
	}
	_, size := utf8.DecodeLastRune(answer)
	clear(answer[len(answer)-size:])
	return answer[:len(answer)-size]
}

func writeMaskedPromptFeedback(output io.Writer, answer []byte, displayedRunes *int) error {
	// UTF-8は複数回のReadで届くため、ひとつのRuneが揃うまで表示数を変えない。
	if !utf8.Valid(answer) {
		return nil
	}
	next := utf8.RuneCount(answer)
	switch {
	case next > *displayedRunes:
		if _, err := io.WriteString(output, strings.Repeat("*", next-*displayedRunes)); err != nil {
			return err
		}
	case next < *displayedRunes:
		if _, err := io.WriteString(output, strings.Repeat("\b \b", *displayedRunes-next)); err != nil {
			return err
		}
	}
	*displayedRunes = next
	return nil
}

// InputBuffer は、握手のあいだに打たれたバイト列を溜める。
//
// io.Pipe ではない。あれは書き込みが読み取りを待つ。問いが出ていない間に
// 打たれた文字で WebSocket の読み手が止まり、その接続全体が固まる。ここは
// 書き込みが決して待たず、上限を超えたぶんは捨てる。溜めるのはユーザーが打った
// 数十バイトであって、際限なく増えるものではない。
type InputBuffer struct {
	mutex  sync.Mutex
	ready  *sync.Cond
	data   []byte
	closed bool
	// gatedはSSHのReady前だけ有効にする。認証promptが待っている間の回答だけを
	// 受け付け、通常の打鍵を新しいshellへ持ち越さない。
	gated   bool
	usable  bool
	prompts int
}

// MaxBufferedInput は、問いが出ていない間に溜める量の上限である。
const MaxBufferedInput = 4 << 10

func NewInputBuffer() *InputBuffer {
	buffer := &InputBuffer{}
	buffer.ready = sync.NewCond(&buffer.mutex)
	return buffer
}

// Write は決して待たない。上限を超えたぶんは捨てる。
func (b *InputBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	if b.gated && !b.usable && b.prompts == 0 {
		// 端末のwriterは入力を再送できないため、拒否ではなく消費済みとして捨てる。
		return len(p), nil
	}
	if room := MaxBufferedInput - len(b.data); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		b.data = append(b.data, p[:room]...)
		b.ready.Broadcast()
	}
	// 捨てた分も書けたことにする。書き手は端末の入力であり、溢れたことを
	// 伝える先が無い。伝えられるのはこの接続が固まることだけである。
	return len(p), nil
}

// WriteExact waits until one complete frame fits and then enqueues it in a
// single critical section. Broadcast commands use this path because the normal
// keystroke writer deliberately reports overflow as consumed.
func (b *InputBuffer) WriteExact(ctx context.Context, p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if len(p) > MaxBufferedInput {
		return errors.New("ssh input frame exceeds the exact-input buffer")
	}
	stop := context.AfterFunc(ctx, func() {
		b.mutex.Lock()
		b.ready.Broadcast()
		b.mutex.Unlock()
	})
	defer stop()

	b.mutex.Lock()
	defer b.mutex.Unlock()
	for MaxBufferedInput-len(b.data) < len(p) && !b.closed && ctx.Err() == nil {
		b.ready.Wait()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.closed {
		return io.ErrClosedPipe
	}
	b.data = append(b.data, p...)
	b.ready.Broadcast()
	return nil
}

// enablePromptGateは、このbufferを非同期SSH handshake用にする。
func (b *InputBuffer) enablePromptGate() {
	b.mutex.Lock()
	b.gated = true
	b.mutex.Unlock()
}

func (b *InputBuffer) beginPrompt() {
	b.mutex.Lock()
	b.prompts++
	b.mutex.Unlock()
}

func (b *InputBuffer) endPrompt() {
	b.mutex.Lock()
	if b.prompts > 0 {
		b.prompts--
	}
	b.mutex.Unlock()
}

func (b *InputBuffer) markUsable() {
	b.mutex.Lock()
	b.usable = true
	b.ready.Broadcast()
	b.mutex.Unlock()
}

func (b *InputBuffer) awaitingPrompt() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return !b.closed && !b.usable && b.prompts > 0
}

// Read は、バイトが来るか閉じられるまで待つ。
func (b *InputBuffer) Read(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for len(b.data) == 0 {
		if b.closed {
			return 0, io.EOF
		}
		b.ready.Wait()
	}
	read := copy(p, b.data)
	b.data = b.data[read:]
	b.ready.Broadcast()
	return read, nil
}

// Close は、待っている読み手を EOF で起動する。
func (b *InputBuffer) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.closed = true
	b.ready.Broadcast()
	return nil
}

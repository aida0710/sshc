package sshclient

import (
	"errors"
	"io"
	"strings"
	"sync"
)

// ErrPromptAborted は、人が問いに答えずに打ち切ったことを報告する。
var ErrPromptAborted = errors.New("the prompt was cancelled")

// Prompter は、接続の途中で人に尋ねる手段である。
//
// 尋ねることは 4 つある——未知のホスト鍵を受け入れるか、鍵のパスフレーズ、
// パスワード、keyboard-interactive の質問。**仕組みを 4 つ作らない。**
// どれも「端末へ書いて、端末から読む」という同じことである。
type Prompter interface {
	// Line は打った文字が見える問い。
	Line(prompt string) (string, error)
	// Secret は打った文字が見えない問い。
	Secret(prompt string) (string, error)
	// Confirm は yes か no を求める。
	Confirm(prompt string) (bool, error)
}

// StreamPrompter は、端末のストリームへ問いを出す。
//
// 端末は raw モードである（xterm.js はローカルエコーを持たない）。だから
// 見える問いのエコーはこちらが書く。見えない問いでは書かない——**答えを
// 端末へ書き戻さないのは、それが画面にもスクロールバックにも残るからである。**
type StreamPrompter struct {
	Out io.Writer
	In  io.Reader
}

func (p StreamPrompter) Line(prompt string) (string, error)   { return p.read(prompt, true) }
func (p StreamPrompter) Secret(prompt string) (string, error) { return p.read(prompt, false) }

// Confirm は yes か no だけを受ける。
//
// OpenSSH と同じで、y も Enter も答えにならない。ホスト鍵を受け入れるかどうかは
// 打ち間違いで通ってよい問いではない。
func (p StreamPrompter) Confirm(prompt string) (bool, error) {
	for attempt := 0; attempt < maxConfirmAttempts; attempt++ {
		answer, err := p.read(prompt, true)
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
// 答えないクライアントがこの接続の goroutine を永久に保持する。
const maxConfirmAttempts = 3

// maxAnswer は、ひとつの答えの長さの上限である。
const maxAnswer = 1024

func (p StreamPrompter) read(prompt string, echo bool) (string, error) {
	if prompt != "" {
		if _, err := io.WriteString(p.Out, prompt); err != nil {
			return "", err
		}
	}

	var answer []byte
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
			// Ctrl-C と Ctrl-D。答えずに打ち切ったという事実である。
			if _, err := io.WriteString(p.Out, "\r\n"); err != nil {
				return "", err
			}
			return "", ErrPromptAborted
		case 0x7f, 0x08:
			if len(answer) == 0 {
				continue
			}
			answer = answer[:len(answer)-1]
			if echo {
				if _, err := io.WriteString(p.Out, "\b \b"); err != nil {
					return "", err
				}
			}
		default:
			if character < 0x20 || len(answer) >= maxAnswer {
				continue
			}
			answer = append(answer, character)
			if echo {
				if _, err := p.Out.Write(buffer); err != nil {
					return "", err
				}
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
	}
}

// InputBuffer は、握手のあいだに打たれたバイト列を溜める。
//
// io.Pipe ではない。**あれは書き込みが読み取りを待つ。** 問いが出ていない間に
// 打たれた文字で WebSocket の読み手が止まり、その接続全体が固まる。ここは
// 書き込みが決して待たず、上限を超えたぶんは捨てる——溜めるのは人が打った
// 数十バイトであって、際限なく増えるものではない。
type InputBuffer struct {
	mutex  sync.Mutex
	ready  *sync.Cond
	data   []byte
	closed bool
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
	return read, nil
}

// Close は、待っている読み手を EOF で起こす。
func (b *InputBuffer) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.closed = true
	b.ready.Broadcast()
	return nil
}

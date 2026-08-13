package sshclient_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"sshc/internal/sshclient"
)

func promptOver(input string) (*strings.Builder, sshclient.StreamPrompter) {
	output := &strings.Builder{}
	return output, sshclient.StreamPrompter{Out: output, In: strings.NewReader(input)}
}

// ブラウザの端末は Enter を CR で送る。LF しか見ないと、答えが永久に終わらない。
func TestALineEndsAtEitherCarriageReturnOrNewline(t *testing.T) {
	for _, ending := range []string{"\r", "\n", "\r\n"} {
		_, prompter := promptOver("ops" + ending)
		answer, err := prompter.Line("login: ")
		if err != nil || answer != "ops" {
			t.Errorf("Line(%q) = %q, %v", ending, answer, err)
		}
	}
}

func TestAVisibleAnswerIsEchoedSoTheUserSeesWhatTheyType(t *testing.T) {
	output, prompter := promptOver("ops\r")
	if _, err := prompter.Line("login: "); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "login: ") {
		t.Errorf("the prompt was not written: %q", output.String())
	}
	if !strings.Contains(output.String(), "ops") {
		t.Errorf("a visible answer was not echoed: %q", output.String())
	}
}

// **答えを端末へ書き戻さない。** 書けば画面にもスクロールバックにも残る。
func TestASecretAnswerIsNeverWrittenBackToTheTerminal(t *testing.T) {
	output, prompter := promptOver("hunter2\r")
	answer, err := prompter.Secret("passphrase: ")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "hunter2" {
		t.Fatalf("Secret() = %q", answer)
	}
	if strings.Contains(output.String(), "hunter2") {
		t.Fatalf("the secret was echoed into the terminal: %q", output.String())
	}
	if !strings.Contains(output.String(), "passphrase: ") {
		t.Errorf("the prompt was not written: %q", output.String())
	}
}

func TestBackspaceRemovesTheLastCharacter(t *testing.T) {
	_, prompter := promptOver("opsx\x7f\r")
	answer, err := prompter.Line("login: ")
	if err != nil || answer != "ops" {
		t.Fatalf("Line() = %q, %v", answer, err)
	}
}

// 空の答えに対する backspace は、その前に打たれた文字を消しに行かない。
func TestBackspaceOnAnEmptyAnswerDoesNothing(t *testing.T) {
	_, prompter := promptOver("\x7f\x7fops\r")
	answer, err := prompter.Line("login: ")
	if err != nil || answer != "ops" {
		t.Fatalf("Line() = %q, %v", answer, err)
	}
}

func TestControlCAbortsTheAnswer(t *testing.T) {
	_, prompter := promptOver("part\x03")
	if _, err := prompter.Line("login: "); !errors.Is(err, sshclient.ErrPromptAborted) {
		t.Fatalf("Line() = %v, want ErrPromptAborted", err)
	}
}

// 入力が閉じられたのは、答えが空だったのではなく、答えられなくなったのである。
func TestAClosedInputAbortsRatherThanAnsweringEmpty(t *testing.T) {
	_, prompter := promptOver("")
	if _, err := prompter.Secret("passphrase: "); !errors.Is(err, sshclient.ErrPromptAborted) {
		t.Fatalf("Secret() = %v, want ErrPromptAborted", err)
	}
}

// yes と no だけを受ける。OpenSSH と同じで、y も Enter も答えにならない——
// ホスト鍵を受け入れるかどうかは、打ち間違いで通ってよい問いではない。
func TestConfirmAcceptsOnlyTheWholeWords(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
		fail  bool
	}{
		{input: "yes\r", want: true},
		{input: "YES\r", want: true},
		{input: "no\r", want: false},
		{input: "y\r\nyes\r", want: true},
		{input: "y\r\n\r\nmaybe\r", fail: true},
	} {
		_, prompter := promptOver(test.input)
		got, err := prompter.Confirm("continue (yes/no)? ")
		switch {
		case test.fail && err == nil:
			t.Errorf("Confirm(%q) = %v, want a refusal", test.input, got)
		case !test.fail && err != nil:
			t.Errorf("Confirm(%q) = %v", test.input, err)
		case !test.fail && got != test.want:
			t.Errorf("Confirm(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

// **書き込みが読み取りを待ってはならない。** io.Pipe なら、問いが出ていない
// 間に打たれた文字ひとつで WebSocket の読み手が止まり、その接続全体が固まる。
func TestTheInputBufferNeverBlocksItsWriter(t *testing.T) {
	buffer := sshclient.NewInputBuffer()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; index < 100; index++ {
			if _, err := buffer.Write([]byte("nobody is reading this\r")); err != nil {
				t.Errorf("Write = %v", err)
				return
			}
		}
	}()
	<-done

	// 溜まっているぶんは読める。上限を超えたぶんは捨てられている。
	read := make([]byte, sshclient.MaxBufferedInput*2)
	count, err := buffer.Read(read)
	if err != nil || count == 0 {
		t.Fatalf("Read = %d, %v", count, err)
	}
	if count > sshclient.MaxBufferedInput {
		t.Fatalf("the buffer grew past its ceiling: %d bytes", count)
	}
}

func TestTheInputBufferWakesItsReaderWhenBytesArrive(t *testing.T) {
	buffer := sshclient.NewInputBuffer()
	var wait sync.WaitGroup
	wait.Add(1)
	var answer string
	go func() {
		defer wait.Done()
		prompter := sshclient.StreamPrompter{Out: io.Discard, In: buffer}
		got, err := prompter.Line("login: ")
		if err != nil {
			t.Errorf("Line = %v", err)
			return
		}
		answer = got
	}()

	if _, err := buffer.Write([]byte("ops\r")); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	if answer != "ops" {
		t.Fatalf("answer = %q", answer)
	}
}

func TestAClosedInputBufferEndsTheWait(t *testing.T) {
	buffer := sshclient.NewInputBuffer()
	done := make(chan error, 1)
	go func() {
		prompter := sshclient.StreamPrompter{Out: io.Discard, In: buffer}
		_, err := prompter.Secret("passphrase: ")
		done <- err
	}()

	if err := buffer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, sshclient.ErrPromptAborted) {
		t.Fatalf("a closed input = %v, want ErrPromptAborted", err)
	}
}

package snippets

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 手順のスニペットは、接続時の自動実行で、シェルの出力に決まった文字列が出るのを待ってから打ち込む。
// sudo や enable のように、問いが出てから次を送る接続先のためである。待つのは認証が済んだあとの
// シェルの出力だけなので（terminal.ShellOutput）、認証の問いへ答えることはない。

const (
	// MaxSteps は、1つのスニペットに並べられる手順の数の上限である。
	MaxSteps = 32
	// MaxWaitBytes は、待つ文字列の長さの上限である。待つのはプロンプトの終わりのような短い文字列である。
	MaxWaitBytes = 256
	// MaxStepSendBytes は、1つの手順で送る文字列の上限である。1回の入力として欠けずに送れる大きさ
	// （terminal.MaxCommandBytes）にそろえる。
	MaxStepSendBytes = (4 << 10) - 1
	// DefaultWaitSeconds と MaxWaitSeconds は、待つ文字列が出るまで待つ秒数の既定と上限である。
	DefaultWaitSeconds = 10
	MaxWaitSeconds     = 300
)

// ErrStepsOnlyAtStartup は、手順のスニペットを、接続時の自動実行のほか（複数の接続先での実行、
// ターミナルへの送信）で使おうとしたことを表す。手順は接続した直後のシェルの出力を待つものである。
var ErrStepsOnlyAtStartup = errors.New("a snippet of steps runs only when a connection starts")

// Step is one step of a snippet of steps. When Wait is not empty, the step first waits for it in the shell's output;
// then it types Send, and Enter after it when Enter is set. Send may use {{variables}}; Wait is taken as written.
type Step struct {
	Wait  string `json:"wait,omitempty"`
	Send  string `json:"send"`
	Enter bool   `json:"enter"`
	// Optional steps are passed over when their text does not come in time. Any other step stops the steps there.
	Optional bool `json:"optional,omitempty"`
}

// PreparedStartup is what to type into a shell once its connection is ready: Steps in order, each waiting at most Wait
// for its text.
type PreparedStartup struct {
	Steps []PreparedStep
	Wait  time.Duration
	// Secrets are the values of the secret variables in Steps, which the shell may echo back.
	Secrets []string
}

// PreparedStep is a Step with its variables put in.
type PreparedStep struct {
	Wait     string
	Send     string
	Enter    bool
	Optional bool
}

// waitDuration is how long each step of snippet waits for its text.
func waitDuration(snippet Snippet) time.Duration {
	if snippet.WaitSeconds == 0 {
		return DefaultWaitSeconds * time.Second
	}
	return time.Duration(snippet.WaitSeconds) * time.Second
}

// validateStepShape checks what the steps of snippet are made of, apart from their variables.
func validateStepShape(snippet Snippet) error {
	if snippet.Command != "" || len(snippet.Steps) > MaxSteps || snippet.WaitSeconds < 0 || snippet.WaitSeconds > MaxWaitSeconds {
		return ErrInvalidSnippet
	}
	for _, step := range snippet.Steps {
		if !validWait(step.Wait) || strings.IndexByte(step.Send, 0) >= 0 || len(step.Send) > MaxStepSendBytes || (step.Send == "" && !step.Enter) {
			return ErrInvalidSnippet
		}
	}
	return nil
}

// validWait accepts a text that can appear on a terminal line: no control characters, and not only spaces, which
// every prompt would match.
func validWait(wait string) bool {
	if len(wait) > MaxWaitBytes || !utf8.ValidString(wait) {
		return false
	}
	for _, character := range wait {
		if unicode.IsControl(character) {
			return false
		}
	}
	return wait == "" || strings.TrimSpace(wait) != ""
}

// expandSteps puts the inputs into the texts the steps of snippet send.
func expandSteps(snippet Snippet, inputs map[string]string) ([]PreparedStep, error) {
	if err := validateStepShape(snippet); err != nil {
		return nil, err
	}
	sends := make([]string, len(snippet.Steps))
	for index, step := range snippet.Steps {
		sends[index] = step.Send
	}
	expanded, err := expandTexts(sends, MaxStepSendBytes, snippet.Variables, inputs)
	if err != nil {
		return nil, err
	}
	prepared := make([]PreparedStep, len(snippet.Steps))
	for index, step := range snippet.Steps {
		prepared[index] = PreparedStep{Wait: step.Wait, Send: expanded[index].command, Enter: step.Enter, Optional: step.Optional}
	}
	return prepared, nil
}

// prepareStartup is what a connection types for snippet with inputs: one step that types the command, or its steps.
func prepareStartup(snippet Snippet, inputs map[string]string) (PreparedStartup, error) {
	secrets := secretValues(snippet.Variables, inputs)
	if len(snippet.Steps) == 0 {
		// A command does not wait, so it has no time to wait.
		if snippet.WaitSeconds != 0 {
			return PreparedStartup{}, ErrInvalidSnippet
		}
		expanded, err := expand(snippet.Command, snippet.Variables, inputs)
		if err != nil {
			return PreparedStartup{}, err
		}
		return PreparedStartup{Steps: []PreparedStep{{Send: expanded.command, Enter: true}}, Secrets: secrets}, nil
	}
	steps, err := expandSteps(snippet, inputs)
	if err != nil {
		return PreparedStartup{}, err
	}
	return PreparedStartup{Steps: steps, Wait: waitDuration(snippet), Secrets: secrets}, nil
}

package connectionattempt

import (
	"context"
	"errors"

	"sshc/internal/terminal"
)

// 接続を見ている人（SFTP のタブ）は、known_hosts に無いホスト鍵を受け入れるかを、画面の
// ボタンで答える。ターミナルの問いと同じく、問いは接続の段階（Progress）が確かめている鍵に
// ついてのものであり、その鍵のフィンガープリントを添えた答えだけを受け取る。

var (
	// ErrNotFound は、その alias と識別子の試行を預かっていないことを表す。
	ErrNotFound = errors.New("no such connection attempt")
	// ErrNoHostKeyQuestion は、その鍵についての問いを待っていない試行に答えようとしたことを
	// 表す。答える前に別の答えが届いたか、接続が先へ進んだか、終わった。
	ErrNoHostKeyQuestion = errors.New("the connection attempt is not asking about this host key")
)

// openHostKeyQuestion は、答えを待っているホスト鍵の問いひとつである。
type openHostKeyQuestion struct {
	// words は問いの文（sshclient.UnknownHostPrompt）で、尋ねている鍵のフィンガープリントを含む。
	words string
	// answer は、答えを ConfirmHostKey へ渡す。答えは1度しか送らないので、1つ分の余地があれば
	// 送る側は待たない。
	answer chan bool
}

// ConfirmHostKey は、known_hosts に無いホスト鍵を受け入れるかをこの試行を見ている人に尋ね、
// 答えか ctx の取り消しを待つ。sshclient.ConnectionObserver の ConfirmHostKey に渡す。
//
// 待つ長さに上限は置かない。ターミナルの問いと同じく、答えるか、尋ねた要求が終わる（タブが
// 別の接続先へ移った、画面を閉じた）まで待つ。
func (a *Attempt) ConfirmHostKey(ctx context.Context, words string) (bool, error) {
	question := &openHostKeyQuestion{words: words, answer: make(chan bool, 1)}
	a.registry.mutex.Lock()
	a.hostKeyQuestion = question
	a.registry.mutex.Unlock()
	defer a.closeHostKeyQuestion(question)
	select {
	case accept := <-question.answer:
		return accept, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (a *Attempt) closeHostKeyQuestion(question *openHostKeyQuestion) {
	a.registry.mutex.Lock()
	defer a.registry.mutex.Unlock()
	if a.hostKeyQuestion == question {
		a.hostKeyQuestion = nil
	}
}

// hostKeyQuestionLocked は、この試行が答えを待っているホスト鍵の問いを返す。registry の
// mutex を持って呼ぶ。
func (a *Attempt) hostKeyQuestionLocked() (terminal.HostKeyQuestion, bool) {
	if a.hostKeyQuestion == nil || a.progress == nil {
		return terminal.HostKeyQuestion{}, false
	}
	return a.progress.HostKeyQuestionAbout(a.hostKeyQuestion.words)
}

// AnswerHostKey は、alias への識別子 id の試行が、このフィンガープリントの鍵についての問いを
// 待っていれば、その問いに答える。待っていなければ何もせずに ErrNoHostKeyQuestion を返す。
//
// 問いを確かめるのと答えるのを同じロックの中でするのは、そのあいだに問いが閉じて次の問い
// （ProxyJump の次の段の鍵）が開くと、古い鍵を見て押した答えがそちらへ入るからである。
func (r *Registry) AnswerHostKey(alias, id, fingerprint string, accept bool) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	attempt, ok := r.attempts[attemptKey{alias: alias, id: id}]
	if !ok {
		return ErrNotFound
	}
	question, asking := attempt.hostKeyQuestionLocked()
	if !asking || question.Fingerprint != fingerprint {
		return ErrNoHostKeyQuestion
	}
	attempt.hostKeyQuestion.answer <- accept
	// 答えた問いはここで閉じる。続けて届いた2度目の答えは、どの問いにも入らない。
	attempt.hostKeyQuestion = nil
	return nil
}

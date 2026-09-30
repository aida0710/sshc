package sshclient

import (
	"context"
	"sync"
	"time"
)

// connectDeadline は、ひとつのホップの ConnectTimeout を数える時計である。
//
// OpenSSH の ConnectTimeout は接続と鍵交換に掛かり、認証には掛からない。認証中は
// 利用者の入力のほか、サーバーの側（Duo のプッシュ承認、PAM の失敗時の遅延）や
// エージェントの側（Touch ID、ssh-add -c の確認）でも待つからである。そこで最初の
// 認証方式を試す時点で時計を止め、以後は親の ctx の取り消しと Close だけで止める。
//
// 認証の前に尋ねるのは未知のホスト鍵の確認だけである。尋ねているあいだは時計を
// 止め、答えたら同じ長さで数え直す。入力待ちは Ctrl-C、入力の終わり、セッションを
// 閉じる操作で終わるので、止めても無期限にはならない。
type connectDeadline struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	timeout time.Duration

	mutex   sync.Mutex
	timer   *time.Timer
	prompts int
	// authenticating は、最初の認証方式を試したあとである。数え直さない。
	authenticating bool
}

// startConnectDeadline は、今から first だけ数える。期限を過ぎると ctx を
// context.DeadlineExceeded を理由に取り消す。親の取り消しは入力待ちのあいだも効く。
//
// first は、接続に使った残りの時間である。入力のあとに数え直すときは timeout を使う。
func startConnectDeadline(parent context.Context, first, timeout time.Duration) *connectDeadline {
	ctx, cancel := context.WithCancelCause(parent)
	deadline := &connectDeadline{ctx: ctx, cancel: cancel, timeout: timeout}
	deadline.timer = time.AfterFunc(first, func() { cancel(context.DeadlineExceeded) })
	return deadline
}

// stop は時計を止め、ctx を手放す。
func (d *connectDeadline) stop() {
	d.timer.Stop()
	d.cancel(context.Canceled)
}

func (d *connectDeadline) pause() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.prompts++
	if d.prompts == 1 {
		d.timer.Stop()
	}
}

func (d *connectDeadline) resume() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.prompts == 0 {
		return
	}
	d.prompts--
	if d.prompts == 0 && !d.authenticating {
		d.timer.Reset(d.timeout)
	}
}

// stopAtAuthentication は、方式を試すたびに呼ばれる observe を包み、最初の方式を
// 試す時点で時計を止める。
func (d *connectDeadline) stopAtAuthentication(observe func(method string)) func(method string) {
	return func(method string) {
		d.authenticationStarted()
		if observe != nil {
			observe(method)
		}
	}
}

func (d *connectDeadline) authenticationStarted() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.authenticating = true
	d.timer.Stop()
}

// pausing は、尋ねているあいだ時計を止める Prompter を返す。
//
// 尋ねない Prompter はそのまま返す。Auth.Methods は非対話であることを型で見分け、
// nil を「対話の方式を組み立てない」の意味に使う。包むとその区別が消える。
func (d *connectDeadline) pausing(prompt Prompter) Prompter {
	switch prompt.(type) {
	case nil, nonInteractivePrompter:
		return prompt
	}
	return deadlinePausingPrompter{prompter: prompt, deadline: d}
}

type deadlinePausingPrompter struct {
	prompter Prompter
	deadline *connectDeadline
}

func (p deadlinePausingPrompter) Line(prompt string) (string, error) {
	p.deadline.pause()
	defer p.deadline.resume()
	return p.prompter.Line(prompt)
}

func (p deadlinePausingPrompter) Secret(prompt string) (string, error) {
	p.deadline.pause()
	defer p.deadline.resume()
	return p.prompter.Secret(prompt)
}

func (p deadlinePausingPrompter) Confirm(prompt string) (bool, error) {
	p.deadline.pause()
	defer p.deadline.resume()
	return p.prompter.Confirm(prompt)
}

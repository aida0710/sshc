package remotesync

import "time"

// pushSchedule は、Run が待つ送信の期限を 1 つだけ持つ。期限を動かすときは新しい
// timer を作らず、同じ timer を掛け直す。
type pushSchedule struct {
	// start は、最初の期限を掛けるときに timer を作る（Auto.newPushTimer）。
	start func(delay time.Duration) pushTimer
	timer pushTimer
	// fired は、期限が掛かっているあいだだけ timer の channel を指す。掛かって
	// いなければ nil で、Run の select はこの case を選ばない。
	fired <-chan time.Time
}

// reset は、今から delay 後に期限を掛け直す。まだ発火していない前の期限は捨てる。
func (s *pushSchedule) reset(delay time.Duration) {
	if s.timer == nil {
		s.timer = s.start(delay)
	} else {
		s.timer.Reset(delay)
	}
	s.fired = s.timer.Fired()
}

// clear は、発火を受け取った期限を外す。
func (s *pushSchedule) clear() {
	s.fired = nil
}

func (s *pushSchedule) stop() {
	if s.timer != nil {
		s.timer.Stop()
	}
}

// pushTimer は、Run が送信の期限を待つ timer である。本番は time.Timer を包み
// （systemPushTimer）、テストは SetClockForTest の時計と一緒に進めて自分で発火させる。
type pushTimer interface {
	// Fired は、期限が来たときに値が届く channel である。
	Fired() <-chan time.Time
	// Reset は、今から delay 後に期限を掛け直す。まだ発火していない前の期限は捨てる。
	Reset(delay time.Duration)
	Stop()
}

// systemPushTimer は、time.Timer を pushTimer にする。
type systemPushTimer struct {
	timer *time.Timer
}

func newSystemPushTimer(delay time.Duration) pushTimer {
	return systemPushTimer{timer: time.NewTimer(delay)}
}

func (t systemPushTimer) Fired() <-chan time.Time { return t.timer.C }

// Reset は time.Timer を掛け直す。go.mod が Go 1.23 以降なので、Reset のあとに前の
// 期限の発火を受け取ることはない。
func (t systemPushTimer) Reset(delay time.Duration) { t.timer.Reset(delay) }

func (t systemPushTimer) Stop() { t.timer.Stop() }

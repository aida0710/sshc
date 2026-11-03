package sshclient

import (
	"time"

	"sshc/internal/terminal"
)

// typedPasswordLifetime は、答えのないパスワードをエンジンのメモリに持ち続ける長さである。答えを
// 待つあいだは平文でしかなく、放っておかれた問いのために持ち続けない。10分は、接続のあとに席を
// 外していても戻って答えられる長さである。消えたら、次に打って接続したときにまた聞く。
const typedPasswordLifetime = 10 * time.Minute

// typedPasswordRecorder は、ひとつの接続のあいだ、行き先で打ったパスワードを覚える。
// 通ったかが分かるのは接続が終わったあとなので、打つたびに入れ替える（打ち直したら最後の値が
// 通った値である）。ホップの答えは問いの順に来るので、ロックは要らない。
type typedPasswordRecorder struct {
	alias, binding string
	password       string
	known          bool
}

func newTypedPasswordRecorder(destination Target) *typedPasswordRecorder {
	return &typedPasswordRecorder{alias: destination.Alias, binding: destination.AuthenticationBinding()}
}

// isDestination は、target がこの接続の行き先かを返す。踏み台で打ったパスワードは、
// 「この接続先のパスワード」ではない。
func (r *typedPasswordRecorder) isDestination(target Target) bool {
	return r != nil && target.Alias == r.alias && target.AuthenticationBinding() == r.binding
}

func (r *typedPasswordRecorder) note(target Target, password string) {
	if r.isDestination(target) && password != "" {
		r.password, r.known = password, true
	}
}

// forget は、パスワード以外の方式を試し始めたときに、打った値を忘れる。その方式で通れば、打った
// 値は断られたものである。
func (r *typedPasswordRecorder) forget(target Target) {
	if r.isDestination(target) {
		r.password, r.known = "", false
	}
}

// accepted は、接続のあとで呼ぶ。打った値があれば、それが通ったパスワードである。
func (r *typedPasswordRecorder) accepted() (string, bool) {
	if r == nil {
		return "", false
	}
	return r.password, r.known
}

// passwordSaveOffer は、保存するかの答えを待っているパスワードと、それを忘れる時計である。
type passwordSaveOffer struct {
	typed  terminal.TypedPassword
	expiry *time.Timer
}

func (s *Session) offerPasswordSave(typed terminal.TypedPassword) {
	s.ForgetTypedPassword()
	offer := &passwordSaveOffer{typed: typed}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.closed {
		return
	}
	offer.expiry = time.AfterFunc(typedPasswordLifetime, s.ForgetTypedPassword)
	s.passwordSave = offer
}

// TypedPassword は、保存するかの答えを待っているパスワードを返す。
func (s *Session) TypedPassword() (terminal.TypedPassword, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.passwordSave == nil {
		return terminal.TypedPassword{}, false
	}
	return s.passwordSave.typed, true
}

// ForgetTypedPassword は、答えが出た、セッションが終わった、待つのをやめたときに、パスワードを
// 手放す。
func (s *Session) ForgetTypedPassword() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.passwordSave != nil {
		s.passwordSave.expiry.Stop()
		s.passwordSave = nil
	}
}

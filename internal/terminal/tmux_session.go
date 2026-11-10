package terminal

import (
	"context"
	"strconv"
	"strings"
)

// tmuxSessionPrefix は、sshc が開く tmux のセッションの名前の先頭である。利用者がサーバーで作ったセッションと
// 見分けるために付ける。
const tmuxSessionPrefix = "sshc-"

// TmuxResuming は、すでに動いていた tmux のセッションにつながったかもしれない Process が実装する。そこで
// 走っているのは利用者が残したプログラムなので、何も打ち込まず、新しいシェルとしても扱わない。
type TmuxResuming interface{ ResumedTmuxSession() bool }

func resumedTmuxSession(process Process) bool {
	resuming, ok := process.(TmuxResuming)
	return ok && resuming.ResumedTmuxSession()
}

// tmuxSessionName は、接続の tmux のセッションの名前である。tmux が自分で変える文字（`.` と `:`）と、シェルが
// 解釈する文字は `_` にする。そのため、名前は引用符なしでコマンドに書ける。
func tmuxSessionName(alias string) string {
	return tmuxSessionPrefix + strings.Map(func(character rune) rune {
		if isTmuxSessionNameCharacter(character) {
			return character
		}
		return '_'
	}, alias)
}

func isTmuxSessionNameCharacter(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' || character == '-' || character == '_'
}

// ValidTmuxSessionName は、name が、sshc が tmux のセッションに付ける名前か、空であるかを返す。記録から読んだ
// 名前は、サーバーのコマンドに引用符なしで書くので、それ以外は断る。
func ValidTmuxSessionName(name string) bool {
	if name == "" {
		return true
	}
	return strings.HasPrefix(name, tmuxSessionPrefix) && strings.IndexFunc(name, func(character rune) bool { return !isTmuxSessionNameCharacter(character) }) < 0
}

// claimTmuxSessionLocked は、開こうとしているセッションの tmux のセッションの名前を選ぶ。エンジンの再起動の
// 前に持っていた名前が空いていればそれを、そうでなければ、その接続の空いている最初の名前を選ぶ。同じ接続への
// 2つのセッションが tmux のセッションを共有すると、同じ画面が出てしまう。呼び出し側が r.mutex を持つ。
// 名前は、セッションが一覧に載るまで、r.pendingTmuxSessions で取られたままになる。
func (r *Registry) claimTmuxSessionLocked(spec Spec) string {
	if spec.Kind != KindSSH || spec.Alias == "" {
		return ""
	}
	taken := make(map[string]bool, len(r.sessions)+len(r.pendingTmuxSessions))
	for _, session := range r.sessions {
		if !session.Discarded() {
			taken[session.tmuxSession] = true
		}
	}
	for _, name := range r.pendingTmuxSessions {
		taken[name] = true
	}
	if spec.TmuxSession != "" && !taken[spec.TmuxSession] {
		return spec.TmuxSession
	}
	base := tmuxSessionName(spec.Alias)
	name := base
	for number := 2; taken[name]; number++ {
		name = base + "-" + strconv.Itoa(number)
	}
	return name
}

type tmuxSessionKey struct{}

// WithTmuxSession は、セッションの tmux のセッションの名前を、その接続を開くコードへ運ぶ。接続が tmux を
// 使うかは、そのコードが決める。
func WithTmuxSession(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, tmuxSessionKey{}, name)
}

// TmuxSessionIn は、ctx が運ぶ tmux のセッションの名前を返す。無ければ ""。
func TmuxSessionIn(ctx context.Context) string {
	name, _ := ctx.Value(tmuxSessionKey{}).(string)
	return name
}

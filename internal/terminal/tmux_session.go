package terminal

import (
	"context"
	"strconv"
	"strings"
)

// tmuxSessionPrefix starts the name of every tmux session sshc opens, so they
// can be told apart from the ones the person made on the server.
const tmuxSessionPrefix = "sshc-"

// TmuxResuming is implemented by a Process that may have attached to a tmux
// session that was already running. What runs there is the program the person
// left, so nothing is typed into it and it is not a new shell.
type TmuxResuming interface{ ResumedTmuxSession() bool }

func resumedTmuxSession(process Process) bool {
	resuming, ok := process.(TmuxResuming)
	return ok && resuming.ResumedTmuxSession()
}

// tmuxSessionName is the name of the tmux session of a connection. Characters
// tmux would change itself (`.` and `:`) or a shell would read become `_`, so
// the name can be written into a command without quoting.
func tmuxSessionName(alias string) string {
	return tmuxSessionPrefix + strings.Map(func(character rune) rune {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9', character == '-', character == '_':
			return character
		}
		return '_'
	}, alias)
}

// claimTmuxSessionLocked picks the tmux session name of a session being opened:
// the one it had before the engine restarted, if that is free, or the first free
// one for its connection. Two sessions to one connection must not share a tmux
// session, or they would show one screen. The caller holds r.mutex, and the name
// stays taken through r.pendingTmuxSessions until the session is listed.
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

// WithTmuxSession carries the tmux session name of a session to the code that
// opens its connection. That code decides whether the connection uses tmux.
func WithTmuxSession(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, tmuxSessionKey{}, name)
}

// TmuxSessionIn returns the tmux session name ctx carries, or "".
func TmuxSessionIn(ctx context.Context) string {
	name, _ := ctx.Value(tmuxSessionKey{}).(string)
	return name
}

// Package terminalrestore keeps the terminal sessions that were open when the
// engine stopped, so the next engine can offer to open them again: after an
// update, a restart of the machine or a manual stop. Sessions themselves live
// only in the engine's memory; what is kept is how to open each one again.
//
// The record belongs to this machine's engine and never travels with sync. It
// holds connection names and directories, never secrets.
package terminalrestore

import (
	"errors"

	"sshc/internal/terminal"
	"sshc/internal/terminalworkspace"
	"sshc/internal/validate"
)

const (
	SchemaVersion = 1
	PathRelative  = "sshc/terminal-sessions.json"
	// MaxSessions bounds what one record may hold. It is far above what a
	// person keeps open, so it only stops a damaged file from growing.
	MaxSessions = 256
	// maxTextBytes bounds every stored string; cwd has the same bound in the API.
	maxTextBytes = 4096
)

var ErrInvalidDocument = errors.New("the saved terminal sessions are invalid")

// Session is how to open one session again.
type Session struct {
	Kind         terminal.Kind `json:"kind"`
	Alias        string        `json:"alias,omitempty"`
	ShellProfile string        `json:"shellProfile,omitempty"`
	Workspace    string        `json:"workspace"`
	PinnedTitle  string        `json:"pinnedTitle,omitempty"`
	// WorkingDirectory is absolute, or under the remote home (`~/src`) for SSH.
	WorkingDirectory string `json:"workingDirectory,omitempty"`
}

// FromReopenings chooses the sessions worth opening again. Sessions in the
// agents workspace are left out: programs open them with `sshc terminal
// create` and open them again themselves.
func FromReopenings(reopenings []terminal.Reopening) []Session {
	sessions := make([]Session, 0, len(reopenings))
	for _, reopening := range reopenings {
		if reopening.Workspace == terminalworkspace.AgentsID || len(sessions) == MaxSessions {
			continue
		}
		sessions = append(sessions, Session{
			Kind: reopening.Kind, Alias: reopening.Alias, ShellProfile: reopening.ShellProfile,
			Workspace: reopening.Workspace, PinnedTitle: reopening.PinnedTitle,
			WorkingDirectory: reopening.WorkingDirectory,
		})
	}
	return sessions
}

func (s Session) valid() bool {
	for _, text := range []string{s.ShellProfile, s.Workspace, s.PinnedTitle, s.WorkingDirectory} {
		if len(text) > maxTextBytes {
			return false
		}
	}
	switch s.Kind {
	case terminal.KindSSH:
		return validate.Alias(s.Alias) == nil
	case terminal.KindShell:
		return s.Alias == ""
	}
	return false
}

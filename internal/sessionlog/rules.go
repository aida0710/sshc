// Package sessionlog keeps the session logs: what each terminal session showed, written as text to a file of its own
// while the session runs. Settings say whether to keep them and for which connections (Rules), and each machine where
// (Directory).
package sessionlog

import (
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

// MaxRulesBytes bounds the rules a person writes; a handful of lines name the connections to leave out.
const MaxRulesBytes = 8 << 10

// LocalShellName is the name a local shell is matched by, as no connection name starts with @.
const LocalShellName = "@local"

// ErrInvalidRules is a rule that is not a pattern.
var ErrInvalidRules = errors.New("the session log rules are not valid")

// Rules say which sessions keep a log, from lines written like a .gitignore: a line is a pattern (* and ?) of the
// connection names whose sessions keep no log, a line that starts with ! names sessions that keep one after all, and a
// later line wins over an earlier one. # starts a comment, and \# and \! a pattern that starts with that character.
// Names are matched without regard to case, as OpenSSH matches Host patterns. With no line, every session keeps a log.
type Rules struct {
	rules []rule
}

type rule struct {
	keeps   bool
	pattern string
}

// CompileRules reads the rules a person wrote.
func CompileRules(text string) (Rules, error) {
	if len(text) > MaxRulesBytes || !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
		return Rules{}, ErrInvalidRules
	}
	var compiled Rules
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keeps := false
		switch {
		case strings.HasPrefix(line, `\#`), strings.HasPrefix(line, `\!`):
			line = line[1:]
		case strings.HasPrefix(line, "!"):
			keeps = true
			line = strings.TrimSpace(line[1:])
		}
		pattern := strings.ToLower(line)
		if _, err := path.Match(pattern, ""); err != nil || pattern == "" {
			return Rules{}, ErrInvalidRules
		}
		compiled.rules = append(compiled.rules, rule{keeps: keeps, pattern: pattern})
	}
	return compiled, nil
}

// Keeps reports whether the session of name keeps a log.
func (r Rules) Keeps(name string) bool {
	name = strings.ToLower(name)
	keeps := true
	for _, rule := range r.rules {
		if matched, _ := path.Match(rule.pattern, name); matched {
			keeps = rule.keeps
		}
	}
	return keeps
}

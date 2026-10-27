package terminal

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// The working directory is where a session is opened again: a restart of the
// engine reopens it there, and a duplicate starts there. Shells report it in
// one of two ways. OSC 7 (`file://host/path`) comes from zsh on macOS, fish
// and many prompt themes. Bash on Debian and Ubuntu sends none, but its
// default title is `user@host: dir`, so that title is read as well.

// maxWorkingDirectoryBytes matches the bound the terminal API puts on cwd.
const maxWorkingDirectoryBytes = 4096

// parseOSC7Directory reads the path of `OSC 7 ; file://host/path`. The host is
// not checked: over SSH it names the remote machine.
func parseOSC7Directory(payload string) (string, bool) {
	location, err := url.Parse(payload)
	if err != nil || location.Scheme != "file" || location.User != nil || location.RawQuery != "" || location.Fragment != "" {
		return "", false
	}
	return checkedWorkingDirectory(location.Path)
}

// workingDirectoryFromTitle reads the directory of a `user@host: dir` title.
// The directory is absolute or under the home directory (`~`, `~/src`).
func workingDirectoryFromTitle(title string) (string, bool) {
	// A title cut at MaxTitle may end in the middle of the path.
	if utf8.RuneCountInString(title) >= MaxTitle {
		return "", false
	}
	identity, directory, found := strings.Cut(title, ": ")
	if !found || strings.ContainsAny(identity, " \t:") {
		return "", false
	}
	user, host, found := strings.Cut(identity, "@")
	if !found || user == "" || host == "" {
		return "", false
	}
	if directory != "~" && !strings.HasPrefix(directory, "~/") && !strings.HasPrefix(directory, "/") {
		return "", false
	}
	return checkedWorkingDirectory(directory)
}

func checkedWorkingDirectory(directory string) (string, bool) {
	if directory == "" || len(directory) > maxWorkingDirectoryBytes || strings.ContainsAny(directory, "\x00\r\n") {
		return "", false
	}
	if directory[0] != '/' && directory[0] != '~' {
		return "", false
	}
	return directory, true
}

// acceptWorkingDirectory records a directory the shell of the current process
// generation reported. It is kept across reconnects: the next process starts
// in the home directory, but the last place the user worked is still the one
// worth reopening.
func (s *Session) acceptWorkingDirectory(generation uint64, directory string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.workingDirectory = directory
}

// WorkingDirectory returns where the session works now, or "" when that is
// unknown. A local process the engine can inspect is asked directly, since
// most local shells report nothing; otherwise this is the last directory the
// shell reported. Over SSH it may begin with `~`.
func (s *Session) WorkingDirectory() string {
	if directory, err := s.ForegroundDirectory(); err == nil {
		return directory
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.kind == KindShell {
		return localWorkingDirectory(s.workingDirectory)
	}
	return s.workingDirectory
}

// localWorkingDirectory turns a reported directory into one this machine can
// start a process in, or "" when it cannot. OSC 7 writes a Windows drive path
// as `/C:/Users`; a title's `~` is a shell's notation, not a path.
func localWorkingDirectory(reported string) string {
	if runtime.GOOS == "windows" && len(reported) >= 3 && reported[0] == '/' && reported[2] == ':' {
		reported = reported[1:]
	}
	if !filepath.IsAbs(filepath.FromSlash(reported)) {
		return ""
	}
	return reported
}

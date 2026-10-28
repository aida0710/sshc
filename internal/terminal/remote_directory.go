package terminal

import (
	"errors"
	"path"
	"strings"
)

var ErrInvalidRemoteWorkingDirectory = errors.New("remote working directory is invalid")

// CheckRemoteWorkingDirectory accepts an absolute POSIX directory on the SSH
// server, or one under the remote home (`~`, `~/src`) as a shell reports it in
// its title.
func CheckRemoteWorkingDirectory(candidate string) (string, error) {
	if !isDirectoryText(candidate) {
		return "", ErrInvalidRemoteWorkingDirectory
	}
	underHome, isUnderHome := strings.CutPrefix(candidate, "~/")
	switch {
	case candidate == "~":
		return candidate, nil
	case isUnderHome:
		if path.Clean(underHome) != underHome || strings.HasPrefix(underHome, "/") || strings.HasPrefix(underHome, "../") || underHome == ".." {
			return "", ErrInvalidRemoteWorkingDirectory
		}
		return candidate, nil
	case strings.HasPrefix(candidate, "/") && path.Clean(candidate) == candidate:
		return candidate, nil
	}
	return "", ErrInvalidRemoteWorkingDirectory
}

// shellSpecialCharacters are the characters that are not plain text inside
// single quotes in every login shell an SSH server may run. sh, bash and zsh
// end the quotes at `'`; fish also reads `\` inside them; PowerShell also ends
// them at the typographic quotes U+2018 to U+201B; cmd.exe has no single quotes
// at all, so its operators and expansions (`&|<>^%!"`) act anywhere. A
// backquote is PowerShell's escape character.
const shellSpecialCharacters = "'\\\"`&|<>^%!‘’‚‛"

// ChangeRemoteDirectoryCommand is the command that moves the remote shell into
// a directory CheckRemoteWorkingDirectory accepted. The `~` is left outside the
// quotes so the shell expands it to the home directory.
//
// The login shell of the server is unknown, so the command is typed only when
// the directory means the same in all of them. Otherwise it is false and the
// session starts in the home directory: a shell's output decides the
// directory, and quoting one shell's way could end the quotes in another and
// run the rest as a command.
func ChangeRemoteDirectoryCommand(directory string) (string, bool) {
	if directory == "~" {
		return "cd", true
	}
	if strings.ContainsAny(directory, shellSpecialCharacters) {
		return "", false
	}
	if underHome, ok := strings.CutPrefix(directory, "~/"); ok {
		return "cd -- ~/'" + underHome + "'", true
	}
	return "cd -- '" + directory + "'", true
}

package sessionlog

import (
	"errors"
	"path/filepath"
	"strings"

	"sshc/internal/platform"
)

// ErrUnsupported is a system where a person could not open the logs: on Android the engine can write only inside the
// app's own folder.
var ErrUnsupported = errors.New("session logs are not kept on this system")

// ErrSynchronizedDirectory is a folder for the logs inside the folder that travels to other machines.
var ErrSynchronizedDirectory = errors.New("session logs cannot be kept in the folder that is synchronized")

// DefaultDirectory is where a machine keeps session logs when its settings name no folder: where its system keeps an
// application's logs or state. It is outside ~/.ssh on purpose: the files there travel to other machines, and the
// terminal output must never be among them. goos is the system (runtime.GOOS), home the person's home folder, and
// environment reads a variable of the engine's.
func DefaultDirectory(goos, home string, environment func(string) string) (string, error) {
	switch goos {
	case "android":
		return "", ErrUnsupported
	case "darwin":
		return filepath.Join(home, "Library", "Logs", "sshc", "sessions"), nil
	case "windows":
		local := environment("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(local, "sshc", "session-logs"), nil
	default:
		state := environment("XDG_STATE_HOME")
		if !filepath.IsAbs(state) {
			state = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(state, "sshc", "session-logs"), nil
	}
}

// Directory is the folder a machine keeps its session logs in: configured, written as an absolute path or under ~,
// or the default when it is empty.
func Directory(configured, goos, home string, environment func(string) string) (string, error) {
	if goos == "android" {
		return "", ErrUnsupported
	}
	if configured == "" {
		return DefaultDirectory(goos, home, environment)
	}
	return platform.ResolveUnderHome(configured, home)
}

// RefuseSynchronized refuses a folder for the logs that is synchronized, or inside it: the terminal output must never
// travel to other machines, and everything under that folder does unless a rule leaves it out. goos decides whether
// names differ by case.
func RefuseSynchronized(directory, synchronized, goos string) error {
	if goos == "windows" || goos == "darwin" {
		directory, synchronized = strings.ToLower(directory), strings.ToLower(synchronized)
	}
	below, err := filepath.Rel(filepath.Clean(synchronized), filepath.Clean(directory))
	if err != nil {
		return nil
	}
	if below == ".." || strings.HasPrefix(below, ".."+string(filepath.Separator)) {
		return nil
	}
	return ErrSynchronizedDirectory
}

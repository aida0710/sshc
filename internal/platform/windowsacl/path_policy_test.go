package windowsacl

import (
	"errors"
	"os"
	"testing"
)

func TestWindowsPrivateDirectoryPathPolicy(t *testing.T) {
	for name, path := range map[string]string{
		"drive descendant":       `C:\Users\aida\.ssh\sshc`,
		"drive slash descendant": `c:/Users/aida/.ssh/sshc`,
		"UNC descendant":         `\\server\share\users\aida\sshc`,
		"UNC slash descendant":   `//server/share/users/aida/sshc`,
	} {
		t.Run("accept "+name, func(t *testing.T) {
			if err := validateWindowsPrivateDirectoryPath(path); err != nil {
				t.Fatalf("validateWindowsPrivateDirectoryPath(%q) = %v", path, err)
			}
		})
	}

	for name, path := range map[string]string{
		"empty":                      "",
		"relative":                   `relative\state`,
		"rooted without volume":      `\state`,
		"drive relative":             `C:state`,
		"drive root":                 `C:\`,
		"drive root after traversal": `C:\state\..`,
		"UNC root":                   `\\server\share\`,
		"UNC root without slash":     `\\server\share`,
		"UNC root after traversal":   `\\server\share\state\..`,
		"extended drive":             `\\?\C:\Users\aida\.ssh\sshc`,
		"extended UNC":               `\\?\UNC\server\share\state`,
		"NT object manager":          `\??\C:\state`,
		"local device":               `\\.\C:\state`,
		"local device root":          `\\.\`,
		"GLOBALROOT extended":        `\\?\GLOBALROOT\Device\HarddiskVolume1\state`,
		"GLOBALROOT UNC spelling":    `\\GLOBALROOT\Device\HarddiskVolume1\state`,
	} {
		t.Run("reject "+name, func(t *testing.T) {
			if err := validateWindowsPrivateDirectoryPath(path); !errors.Is(err, os.ErrInvalid) {
				t.Fatalf("validateWindowsPrivateDirectoryPath(%q) = %v, want os.ErrInvalid", path, err)
			}
		})
	}
}

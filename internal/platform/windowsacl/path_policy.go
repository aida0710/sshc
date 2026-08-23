package windowsacl

import (
	"os"
	"strings"
)

// ValidatePrivatePath は DOS drive の絶対パス配下または通常の UNC share 配下だけを
// 受け付ける。非公開状態のパスに Windows device namespace 表記は許可しない。
func ValidatePrivatePath(path string) error {
	return validateWindowsPrivateDirectoryPath(path)
}

func validateWindowsPrivateDirectoryPath(path string) error {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return os.ErrInvalid
	}
	normalized := strings.ReplaceAll(path, "/", `\`)
	lower := strings.ToLower(normalized)
	for _, prefix := range []string{`\\?\`, `\\.\`, `\??\`} {
		if strings.HasPrefix(lower, prefix) {
			return os.ErrInvalid
		}
	}

	if strings.HasPrefix(normalized, `\\`) {
		parts := strings.Split(strings.TrimPrefix(normalized, `\\`), `\`)
		if len(parts) < 2 || invalidUNCAnchor(parts[0]) || invalidUNCAnchor(parts[1]) {
			return os.ErrInvalid
		}
		if _, ok := cleanWindowsDescendants(parts[2:]); !ok {
			return os.ErrInvalid
		}
		return nil
	}

	if len(normalized) < 3 || !isASCIILetter(normalized[0]) || normalized[1] != ':' || normalized[2] != '\\' {
		return os.ErrInvalid
	}
	if _, ok := cleanWindowsDescendants(strings.Split(normalized[3:], `\`)); !ok {
		return os.ErrInvalid
	}
	return nil
}

func invalidUNCAnchor(component string) bool {
	switch strings.ToLower(component) {
	case "", ".", "..", "?", "??", "globalroot", "device":
		return true
	default:
		return strings.Contains(component, ":")
	}
}

func cleanWindowsDescendants(components []string) ([]string, bool) {
	cleaned := make([]string, 0, len(components))
	for _, component := range components {
		switch component {
		case "", ".":
			continue
		case "..":
			if len(cleaned) == 0 {
				return nil, false
			}
			cleaned = cleaned[:len(cleaned)-1]
		default:
			if strings.Contains(component, ":") {
				return nil, false
			}
			cleaned = append(cleaned, component)
		}
	}
	return cleaned, len(cleaned) > 0
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

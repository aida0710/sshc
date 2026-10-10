package buildcontract

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebModulesRemainUnambiguousOnCaseInsensitiveFilesystems(t *testing.T) {
	sourceDirectory := filepath.Join("..", "..", "web", "src")
	modulePaths := make(map[string]string)
	err := filepath.WalkDir(sourceDirectory, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() || strings.HasSuffix(path, ".d.ts") {
			return nil
		}
		extension := filepath.Ext(path)
		if extension != ".ts" && extension != ".tsx" {
			return nil
		}
		// Extensionless imports resolve both .ts and .tsx, so different
		// extensions do not prevent collisions on macOS and Windows.
		modulePath := strings.ToLower(strings.TrimSuffix(path, extension))
		if previousPath, exists := modulePaths[modulePath]; exists {
			t.Errorf("ambiguous Web modules on a case-insensitive filesystem: %s and %s", previousPath, path)
		}
		modulePaths[modulePath] = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

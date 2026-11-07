package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The SSH config fields of the web UI complete keywords from web/src/editor/sshConfigKeywords.ts. Every one must be a
// keyword OpenSSH knows, or the completion would write a line OpenSSH stops at.
func TestTheKeywordsTheWebUICompletesAreKnownToOpenSSH(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "editor", "sshConfigKeywords.ts"))
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile(`"([A-Za-z0-9]+)"`).FindAllStringSubmatch(string(contents), -1)
	if len(quoted) < 90 {
		t.Fatalf("read %d keywords from the web UI's list, want the whole list", len(quoted))
	}
	for _, match := range quoted {
		if !openSSHKeywords[strings.ToLower(match[1])] {
			t.Errorf("%s is not a keyword OpenSSH knows", match[1])
		}
	}
}

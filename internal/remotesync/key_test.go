package remotesync_test

import (
	"strings"
	"testing"

	"sshc/internal/envelope"
	"sshc/internal/remotesync"
)

func TestNewKeyIsReadableAndStrongEnoughToSealWith(t *testing.T) {
	key, err := remotesync.NewKey()
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}

	if got, want := len(key), 29; got != want {
		t.Fatalf("key %q is %d characters, want %d", key, got, want)
	}
	for _, group := range strings.Split(key, "-") {
		if len(group) != 4 {
			t.Fatalf("key %q has a group that is not four characters: %q", key, group)
		}
	}

	// 読み違いは、鍵を別の端末へ運ぶこの用途では「開かない」になる。
	if index := strings.IndexAny(key, "ILOU"); index >= 0 {
		t.Fatalf("key %q contains a character that reads as another: %q", key, key[index])
	}
	if key != strings.ToUpper(key) {
		t.Fatalf("key %q is not upper case", key)
	}

	// 生成した鍵をそのまま封に使えなければ、生成する意味がない。
	if _, err := envelope.Derive(key); err != nil {
		t.Fatalf("the generated key cannot seal an envelope: %v", err)
	}
}

func TestNewKeyDoesNotRepeatItself(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		key, err := remotesync.NewKey()
		if err != nil {
			t.Fatalf("NewKey: %v", err)
		}
		if seen[key] {
			t.Fatalf("NewKey returned %q twice", key)
		}
		seen[key] = true
	}
}

package sshclient

import (
	"bytes"
	"testing"
)

func TestInputBufferWipesConsumedBytesFromItsBackingArray(t *testing.T) {
	buffer := NewInputBuffer()
	secret := []byte("hunter2\n")
	if _, err := buffer.Write(secret); err != nil {
		t.Fatal(err)
	}
	backing := buffer.data[:cap(buffer.data)]
	out := make([]byte, len(secret))
	if _, err := buffer.Read(out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, secret) {
		t.Fatalf("read %q", out)
	}
	// The reader got the password; the buffer must not keep a second copy.
	if bytes.Contains(backing, []byte("hunter2")) {
		t.Fatal("the consumed password is still in the buffer's backing array")
	}
}

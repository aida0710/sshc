package iowrite

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type writerFunc func([]byte) (int, error)

func (function writerFunc) Write(payload []byte) (int, error) { return function(payload) }

func TestWriteAllContinuesAfterShortWrites(t *testing.T) {
	var written bytes.Buffer
	oneByteAtATime := writerFunc(func(payload []byte) (int, error) {
		return written.Write(payload[:1])
	})
	if err := WriteAll(oneByteAtATime, []byte("hello")); err != nil {
		t.Fatalf("WriteAll = %v", err)
	}
	if written.String() != "hello" {
		t.Fatalf("written = %q, want hello", written.String())
	}
}

func TestWriteAllRejectsAWriteThatMakesNoProgress(t *testing.T) {
	stuck := writerFunc(func([]byte) (int, error) { return 0, nil })
	if err := WriteAll(stuck, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteAll = %v, want io.ErrShortWrite", err)
	}
}

func TestWriteAllRejectsAnImpossibleWriteCount(t *testing.T) {
	for _, count := range []int{-1, 2} {
		impossible := writerFunc(func([]byte) (int, error) { return count, nil })
		if err := WriteAll(impossible, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("WriteAll with count %d = %v, want io.ErrShortWrite", count, err)
		}
	}
}

func TestWriteAllReturnsTheWritersError(t *testing.T) {
	want := errors.New("pipe closed")
	var written bytes.Buffer
	failing := writerFunc(func(payload []byte) (int, error) {
		count, _ := written.Write(payload[:1])
		return count, want
	})
	if err := WriteAll(failing, []byte("ab")); !errors.Is(err, want) {
		t.Fatalf("WriteAll = %v, want %v", err, want)
	}
	if written.String() != "a" {
		t.Fatalf("written = %q, want the byte before the failure", written.String())
	}
}

// 捨てた分も書けたと返す。そうしないと os/exec は写しを止め、プログラム側の書き込みが詰まる。
func TestCappedBufferKeepsUpToTheLimitAndReportsTheWholeWrite(t *testing.T) {
	buffer := NewCappedBuffer(8)
	written, err := buffer.Write([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if written != 16 {
		t.Errorf("Write = %d, want 16", written)
	}
	if buffer.String() != "01234567" {
		t.Errorf("kept %q, want 01234567", buffer.String())
	}
	if !buffer.Truncated() {
		t.Error("Truncated = false after dropping bytes")
	}
}

func TestCappedBufferIsNotTruncatedWhenTheWritesFitExactly(t *testing.T) {
	buffer := NewCappedBuffer(4)
	for _, chunk := range []string{"ab", "cd", ""} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if buffer.Truncated() {
		t.Error("Truncated = true although every byte fit")
	}
	if got := string(buffer.Bytes()); got != "abcd" {
		t.Errorf("Bytes = %q, want abcd", got)
	}
}

func TestCappedBufferBytesIsACopy(t *testing.T) {
	buffer := NewCappedBuffer(4)
	_, _ = buffer.Write([]byte("ab"))
	copied := buffer.Bytes()
	copied[0] = 'z'
	if buffer.String() != "ab" {
		t.Fatalf("changing the returned bytes changed the buffer to %q", buffer.String())
	}
}

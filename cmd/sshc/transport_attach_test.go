package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type shortWriter struct{ bytes.Buffer }

func (writer *shortWriter) Write(payload []byte) (int, error) {
	if len(payload) > 2 {
		payload = payload[:2]
	}
	return writer.Buffer.Write(payload)
}

func TestCopyTransportInputWritesEveryByteAcrossShortWrites(t *testing.T) {
	var output shortWriter
	if err := copyTransportInput(&output, bytes.NewBufferString("abcdef")); !errors.Is(err, io.EOF) {
		t.Fatalf("error = %v", err)
	}
	if got := output.String(); got != "abcdef" {
		t.Fatalf("output = %q", got)
	}
}

func TestCopyTransportInputStopsAtLocalEscapeWithoutSendingIt(t *testing.T) {
	var output bytes.Buffer
	err := copyTransportInput(&output, bytes.NewReader([]byte{'a', 'b', transportEscapeByte, 'c'}))
	if !errors.Is(err, errLocalEscape) {
		t.Fatalf("error = %v", err)
	}
	if got := output.String(); got != "ab" {
		t.Fatalf("output = %q", got)
	}
}

func TestWriteTransportAllRejectsZeroProgress(t *testing.T) {
	err := writeTransportAll(writerFunc(func([]byte) (int, error) { return 0, nil }), []byte("x"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v", err)
	}
}

type writerFunc func([]byte) (int, error)

func (function writerFunc) Write(payload []byte) (int, error) { return function(payload) }

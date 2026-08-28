package textencoding_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"sshc/internal/textencoding"
)

func TestNamesAreCanonicalised(t *testing.T) {
	for given, want := range map[string]textencoding.Name{
		"": textencoding.UTF8, "UTF8": textencoding.UTF8,
		"sjis": textencoding.ShiftJIS, "windows-31j": textencoding.ShiftJIS,
		"euc_jp": textencoding.EUCJP, "jis": textencoding.ISO2022JP,
	} {
		got, err := textencoding.Parse(given)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", given, got, err, want)
		}
	}
	if _, err := textencoding.Parse("guess"); err == nil {
		t.Fatal("an unknown encoding was accepted")
	}
}

func TestShiftJISRoundTripsAcrossSmallReadsAndWrites(t *testing.T) {
	raw := &memoryStream{}
	wrapped, err := textencoding.Wrap(raw, textencoding.ShiftJIS)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"公", "告", "です"} {
		if _, err := wrapped.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if got := raw.written.Bytes(); bytes.Equal(got, []byte("公告です")) || len(got) == 0 {
		t.Fatalf("wire bytes were not Shift_JIS: %x", got)
	}

	raw.read = bytes.NewReader(raw.written.Bytes())
	var decoded bytes.Buffer
	buffer := make([]byte, 1)
	for {
		count, readErr := wrapped.Read(buffer)
		decoded.Write(buffer[:count])
		if readErr != nil {
			if readErr != io.EOF {
				t.Fatal(readErr)
			}
			break
		}
	}
	if got := decoded.String(); got != "公告です" {
		t.Fatalf("decoded = %q", got)
	}
}

func TestWrapperPreservesTransportCapabilities(t *testing.T) {
	raw := &capableStream{memoryStream: memoryStream{}}
	wrapped, err := textencoding.Wrap(raw, textencoding.EUCJP)
	if err != nil {
		t.Fatal(err)
	}
	if err := wrapped.(interface{ DiscardPending(context.Context) error }).DiscardPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := wrapped.(interface{ SetWindowSize(uint16, uint16) error }).SetWindowSize(132, 43); err != nil {
		t.Fatal(err)
	}
	if !raw.discarded || raw.width != 132 || raw.height != 43 {
		t.Fatalf("capabilities were not delegated: %#v", raw)
	}
}

type memoryStream struct {
	read    io.Reader
	written bytes.Buffer
	closed  bool
}

func (stream *memoryStream) Read(buffer []byte) (int, error) {
	if stream.read == nil {
		return 0, io.EOF
	}
	return stream.read.Read(buffer)
}
func (stream *memoryStream) Write(buffer []byte) (int, error) { return stream.written.Write(buffer) }
func (stream *memoryStream) Close() error                     { stream.closed = true; return nil }

type capableStream struct {
	memoryStream
	discarded     bool
	width, height uint16
}

func (stream *capableStream) DiscardPending(context.Context) error {
	stream.discarded = true
	return nil
}
func (stream *capableStream) SetWindowSize(width, height uint16) error {
	stream.width, stream.height = width, height
	return nil
}
